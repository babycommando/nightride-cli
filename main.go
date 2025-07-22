package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/babycommando/rich-go/client"
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
	{name: "Nightride FM", url: "https://stream.nightride.fm/nightride.mp3"},
	{name: "Darksynth", url: "https://stream.nightride.fm/darksynth.mp3"},
	{name: "Chillsynth", url: "https://stream.nightride.fm/chillsynth.mp3"},
	{name: "Datawave", url: "https://stream.nightride.fm/datawave.mp3"},
	{name: "EBSM", url: "https://stream.nightride.fm/ebsm.mp3"},
	{name: "Horrorsynth", url: "https://stream.nightride.fm/horrorsynth.mp3"},
	{name: "Rekt", url: "https://stream.nightride.fm/rekt.mp3"},
	{name: "Rektory", url: "https://stream.nightride.fm/rektory.mp3"},
	{name: "Spacesynth", url: "https://stream.nightride.fm/spacesynth.mp3"},
}

func (s station) Title() string       { return s.name }
func (s station) Description() string { return fmt.Sprintf("%s", s.title) }
func (s station) FilterValue() string { return s.name }
func (s station) id() string {
	key := strings.ToLower(stationKey(s.url))
	return strings.TrimSuffix(key, ".mp3")
}

/* ─────────────  Bubble Tea model  ───────────── */

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
	playingIdx int
	startTime  time.Time
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
	delegate.Styles.SelectedDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB76B"))

	l := list.New(items, delegate, 48, len(items)*2)
	l.Title = "Nightride  –  ↑/↓ move · Enter play/pause · q quit"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return model{
			l:          l,
			playingIdx: 0,
			startTime:  time.Now(),
	}
}

func fetchAllMetaCmd() tea.Cmd { return func() tea.Msg { return fetchAllMeta() } }

func (m model) Init() tea.Cmd {
	return tea.Batch(
		startStreamCmd(0),
		fetchAllMetaCmd(),
		tickMetaLoop(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.stopCurrent()
			return m, tea.Quit
		case "enter":
			idx := m.l.Index()
			if m.playingIdx == idx {
				m.stopCurrent()
				m.playingIdx = -1
				return m, nil
			}
			m.stopCurrent()
			m.playingIdx = idx
			m.startTime  = time.Now()
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
			key := st.id() + ".mp3"
			if meta, ok := msg[key]; ok {
				st.title, st.listeners = meta.title, meta.listeners
				m.l.SetItem(i, st)
				stations[i].title, stations[i].listeners = st.title, st.listeners
	
				// 🔁 Update Discord status *only for current station*
				if i == m.playingIdx {
					// now := time.Now()
					iconKey := strings.ToLower(stationKey(st.url))      // "Darksynth.mp3" → "darksynth.mp3"
					iconKey = strings.TrimSuffix(iconKey, ".mp3")       // → "darksynth"

					if iconKey == "nightride" {
						iconKey = "nrfm"
					}
	
					parts := strings.SplitN(st.title, " – ", 2)
					artist := ""
					track := st.title
					if len(parts) == 2 {
						artist = parts[0]
						track = parts[1]
					}

					err := client.SetActivity(client.Activity{
						Type:       2,
						Details:    "Listening to " + st.name,
						State:      artist,
						LargeImage: iconKey,
						SmallImage: "nrfm",
						LargeText:  track,
						Timestamps: &client.Timestamps{
							Start: &m.startTime,
						},
						Buttons: []*client.Button{
							{
								Label: "Listen to " + st.name,
								Url:   "https://nightride.fm/?station=" + st.id(),
							},
							{
								Label: "Join the Discord",
								Url:   "https://discord.gg/synthwave",
							},
						},
					})
					if err != nil {
						fmt.Println("Discord RPC error:", err)
					}
				}
			}
		}
		return m, tickMetaLoop()
	case errMsg:
		fmt.Println("audio error:", msg)
		return m, nil
	}

	var cmd tea.Cmd
	m.l, cmd = m.l.Update(msg)
	return m, cmd
}

func (m model) View() string {
	header := "⏸  Paused"
	if m.playingIdx != -1 {
		item := m.l.Items()[m.playingIdx].(station)
		header = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).
			Render("▶  " + item.name + " – " + item.title)
	}
	return lipgloss.JoinVertical(lipgloss.Left, asciiStyle, header, "\n", m.l.View())
}

// ─────────────  global audio state  ─────────────
var (
	speakerOnce       sync.Once
	mixerSampleRate   beep.SampleRate
)

/* ─────────────  Audio helpers  ───────────── */

func (m *model) stopCurrent() {
	_ = client.SetActivity(client.Activity{}) // ← clears Discord presence
	speaker.Clear()

	if m.streamer != nil {
			m.streamer.Close()
			m.streamer = nil
	}
	if m.respBody != nil {
			m.respBody.Close()
			m.respBody = nil
	}
}

func dialAndDecode(url string, tries int) (
	decoded beep.StreamSeekCloser,
	format  beep.Format,
	body    io.ReadCloser,
	err     error,
) {
	for i := 0; i < tries; i++ {
			var resp *http.Response
			resp, err = http.Get(url)
			if err != nil {
					time.Sleep(250 * time.Millisecond)
					continue
			}

			decoded, format, err = mp3.Decode(resp.Body)
			if err == nil {
					return decoded, format, resp.Body, nil
			}

			resp.Body.Close()
			time.Sleep(250 * time.Millisecond)
	}
	return nil, beep.Format{}, nil, err
}


func startStreamCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		st := stations[idx]

		decoded, format, body, err := dialAndDecode(st.url, 5)
		if err != nil {
			return errMsg(err)
		}

		speakerOnce.Do(func() {
			mixerSampleRate = format.SampleRate
			speaker.Init(mixerSampleRate, mixerSampleRate.N(time.Second/10))
		})

		playStream := beep.Streamer(decoded)
		if format.SampleRate != mixerSampleRate {
			playStream = beep.Resample(4, format.SampleRate, mixerSampleRate, decoded)
		}

		speaker.Clear()
		speaker.Play(playStream)

		// ↓↓↓ fixed lowercase + nrfm remap
		iconKey := strings.ToLower(stationKey(st.url))
		iconKey = strings.TrimSuffix(iconKey, ".mp3")
		if iconKey == "nightride" {
			iconKey = "nrfm"
		}

		// ↓↓↓ split "Artist – Title" if possible
		parts := strings.SplitN(st.title, " – ", 2)
		artist := ""
		track := st.title
		if len(parts) == 2 {
			artist = parts[0]
			track = parts[1]
		}

		now := time.Now()
		_ = client.SetActivity(client.Activity{
			Type:       2,
			State:      artist,
			Details:    "Listening to " + st.name,
			LargeImage: iconKey,
			SmallImage: "nrfm", // fixed logo
			LargeText:  track,
			Timestamps: &client.Timestamps{
				Start: &now,
			},
			Buttons: []*client.Button{
				{
					Label: "Listen to " + st.name,
					Url:   "https://nightride.fm/?station=" + st.id(),
				},
				{
					Label: "Join the Discord",
					Url:   "https://discord.gg/synthwave",
				},
			},
		})

		return streamHandleMsg{streamer: decoded, body: body}
	}
}

/* ─────────────  Metadata  ───────────── */

type nowPlaying struct {
	Station string `json:"station"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
}

func stationKey(u string) string {
	return u[strings.LastIndex(u, "/")+1:]
}

func fetchAllMeta() tea.Msg {
	const timeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://nightride.fm/meta", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errMsg(err)
	}
	defer resp.Body.Close()

	meta := make(metaAllMsg)                         
	want := len(stations)                           
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return meta
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var entries []nowPlaying
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &entries); err != nil {
			continue
		}

		for _, np := range entries {
			key := np.Station + ".mp3"
			if _, seen := meta[key]; seen {
				continue
			}
			meta[key] = struct {
				title     string
				listeners int
			}{
				title: fmt.Sprintf("%s – %s", np.Artist, np.Title),
				listeners: 0,
			}
		}

		if len(meta) == want {
			return meta
		}
	}

	if err := scanner.Err(); err != nil {
		return errMsg(err)
	}
	return meta
}

func tickMetaLoop() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return fetchAllMeta()
	})
}

/* ─────────────  main  ───────────── */

func main() {
	err := client.Login("1396017162425991279")
	if err != nil {
		fmt.Println("discord rpc error:", err)
	}

	if err := tea.NewProgram(newModel(), tea.WithAltScreen()).Start(); err != nil && err != io.EOF {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
}

