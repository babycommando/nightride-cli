package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

var asciiArt = `
 ..     %@@ .@@@@=   .. 
 .  +@= -@@    -@@@#    
  -@@@@@=@@ ..    #@@-  
  @@= =@@@@ .....  =@@  
 =@@    =@@  .      +@= 
 #@* ..   =  +@@@@@@@@@ 
 =@@  ..   *#  =@@=     
  @@+ .  -@.*@-  =@@@   
  -@@   +@.   @*   #@=  
      =@@@@.  @@@=      
 .  -#@@##@@ =@#%@@-  . 
  --===---=--==--===--  
`
/* ─────────────  Ascii Data  ───────────── */

var asciiStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ff386f")).
	Render(asciiArt)

/* ─────────────  Station Data  ───────────── */

type station struct {
	name, url string
	title     string
	listeners int
}

var stations = []station{
	{name: "Nightride FM", url: "https://stream.nightride.fm/nightride.mp3"},
	{name: "Darksynth",    url: "https://stream.nightride.fm/darksynth.mp3"},
	{name: "Chillsynth",   url: "https://stream.nightride.fm/chillsynth.mp3"},
	{name: "Datawave",     url: "https://stream.nightride.fm/datawave.mp3"},
	{name: "EBSM",         url: "https://stream.nightride.fm/ebsm.mp3"},
	{name: "Horrorsynth",  url: "https://stream.nightride.fm/horrorsynth.mp3"},
	{name: "Rekt",         url: "https://stream.nightride.fm/rekt.mp3"},
	{name: "Rektory",      url: "https://stream.nightride.fm/rektory.mp3"},
	{name: "Spacesynth",   url: "https://stream.nightride.fm/spacesynth.mp3"},
}

/* ─────────────  List implementation  ───────────── */

func (s station) Title() string       { return s.name }
func (s station) Description() string { return fmt.Sprintf("%s  ·  %d🔊", s.title, s.listeners) }
func (s station) FilterValue() string { return s.name }

/* ─────────────  Bubble Tea model  ───────────── */

type (
	metaAllMsg      = map[string]struct{ title string; listeners int }
	errMsg          error
	streamHandleMsg struct {
		streamer beep.StreamSeekCloser
		body     io.Closer
	}
)

type model struct {
	l          list.Model
	playingIdx int // -1 paused

	streamer beep.StreamSeekCloser
	respBody io.Closer
}

func newModel() model {
	items := make([]list.Item, len(stations))
	for i := range stations {
		items[i] = stations[i]
	}
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5F87"))
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB76B"))

	l := list.New(items, delegate, 48, len(items)*2)
	l.Title = "Nightride  –  ↑/↓ move · Enter play/pause · q quit"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return model{l: l, playingIdx: -1}
}

/* ─────────────  Init (autoplay)  ───────────── */

func (m model) Init() tea.Cmd {
	return tea.Batch(
		startStreamCmd(0), // autoplay first station
		fetchAllMeta(),
		tickMetaLoop(),
	)
}

/* ─────────────  Update  ───────────── */

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {

		case "q", "ctrl+c":
			m.stopCurrent()
			return m, tea.Quit

		case "enter":
			idx := m.l.Index()
			if m.playingIdx == idx { // toggle pause
				m.stopCurrent()
				m.playingIdx = -1
				return m, nil
			}
			m.stopCurrent()
			m.playingIdx = idx
			return m, startStreamCmd(idx)

		case "up", "down", "k", "j":
			var cmd tea.Cmd
			m.l, cmd = m.l.Update(msg)
			return m, cmd
		}

	case streamHandleMsg:
		m.streamer, m.respBody = msg.streamer, msg.body
		return m, nil

	case metaAllMsg:
		for i, itm := range m.l.Items() {
			st := itm.(station)
			key := st.url[strings.LastIndex(st.url, "/")+1:]
			if meta, ok := msg[key]; ok {
				st.title, st.listeners = meta.title, meta.listeners
				m.l.SetItem(i, st)
				stations[i].title, stations[i].listeners = st.title, st.listeners
			}
		}
		return m, nil

	case errMsg:
		fmt.Println("audio error:", msg)
		return m, nil
	}

	var cmd tea.Cmd
	m.l, cmd = m.l.Update(msg)
	return m, cmd
}

/* ─────────────  View  ───────────── */

func (m model) View() string {
	header := "⏸  Paused"
	if m.playingIdx != -1 {
		item := m.l.Items()[m.playingIdx].(station)
		header = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).
			Render("▶  " + item.name + " – " + item.title)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		asciiStyle,  // ← show the ASCII at the top
		header,
		"\n",
		m.l.View(),
	)
}

/* ─────────────  Audio helpers  ───────────── */

func (m *model) stopCurrent() {
	if m.streamer != nil {
		speaker.Clear()
		m.streamer.Close()
		m.streamer = nil
	}
	if m.respBody != nil {
		m.respBody.Close()
		m.respBody = nil
	}
}

func startStreamCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		st := stations[idx]

		// connect + decode, retry a few times
		var (
			resp     *http.Response
			streamer beep.StreamSeekCloser
			format   beep.Format
			err      error
		)
		for i := 0; i < 5; i++ {
			resp, err = http.Get(st.url)
			if err != nil {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			streamer, format, err = mp3.Decode(resp.Body)
			if err == nil {
				break
			}
			resp.Body.Close()
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			return errMsg(err)
		}

		speaker.Clear()
		speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))

		done := make(chan struct{})
		speaker.Play(beep.Seq(streamer, beep.Callback(func() { close(done) })))

		// send handles back so model can close them later
		return streamHandleMsg{streamer: streamer, body: resp.Body}
	}
}

/* ─────────────  Metadata  ───────────── */

type iceStatus struct {
	Icestats struct {
		Source []struct {
			ListenURL    string `json:"listenurl"`
			Title        string `json:"title"`
			DisplayTitle string `json:"display-title"`
			Listeners    int    `json:"listeners"`
		} `json:"source"`
	} `json:"icestats"`
}

func fetchAllMeta() tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get("https://stream.nightride.fm/status-json.xsl")
		if err != nil {
			return errMsg(err)
		}
		defer resp.Body.Close()

		var s iceStatus
		if err = json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return errMsg(err)
		}

		meta := make(metaAllMsg)
		for _, src := range s.Icestats.Source {
			key := src.ListenURL[strings.LastIndex(src.ListenURL, "/")+1:]
			title := src.Title
			if title == "" {
				title = src.DisplayTitle
			}
			meta[key] = struct {
				title     string
				listeners int
			}{title, src.Listeners}
		}
		return meta
	}
}

func tickMetaLoop() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return fetchAllMeta()() })
}

/* ─────────────  main  ───────────── */

func main() {
	if err := tea.NewProgram(newModel(), tea.WithAltScreen()).Start(); err != nil && err != io.EOF {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
}
