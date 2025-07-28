package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

/* ─────────────  Bubble Tea model  (player) ───────────── */

type (
	metaAllMsg = map[string]struct {
		title     string
		listeners int
	}
	errMsg          error
	streamHandleMsg struct {
		streamer beep.StreamSeekCloser
		body     io.Closer
	}
)

type model struct {
	l                  list.Model
	playingIdx         int
	startTime          time.Time
	streamer           beep.StreamSeekCloser
	respBody           io.Closer
	barHeights         []int
	ampChan            chan []float64
	easterEgg          bool
	visPeak            float64
	isHorizontalLayout bool
	showHelp           bool
	scrollOffset       int
	listScrollOffset   int
	originalTitles     map[int]string
	scrollStep         int
}

func (m *model) updateSelectorColors(iconKey string) {
	colors, exists := StationColors[iconKey]
	if !exists {
		colors = []string{"#ff386f", "#7d3cff"}
	}
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Copy().
		Foreground(lipgloss.Color(colors[0])).
		BorderLeftForeground(lipgloss.Color(colors[0]))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Copy().
		Foreground(lipgloss.Color(colors[1])).
		BorderLeftForeground(lipgloss.Color(colors[1]))
	m.l.SetDelegate(delegate)
}

func min(a, b int) int { if a < b { return a }; return b }
func max(a, b int) int { if a > b { return a }; return b }

func newModel() model {
	items := make([]list.Item, len(stations))
	for i := range stations {
		items[i] = stations[i]
	}
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 46, len(items)*2-3)
	l.Title = "↑/↓ Navigate · [↵] Play/Pause · [TAB]Chat"
	l.Styles.Title = l.Styles.Title.Copy().
		UnsetBackground().
		MarginTop(2).
		Margin(0, 0).
		Padding(0, 0).
		Foreground(lipgloss.Color("#CCCCCC")).
		Faint(true)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	originalTitles := make(map[int]string)
	for i := range stations {
		originalTitles[i] = stations[i].title
	}

	m := model{
		l:                  l,
		playingIdx:         0,
		startTime:          time.Now(),
		barHeights:         make([]int, asciiArtWidth()),
		ampChan:            make(chan []float64, 1),
		visPeak:            0.25,
		isHorizontalLayout: true,
		showHelp:           false,
		originalTitles:     originalTitles,
		scrollStep:         1,
	}

	if len(stations) > 0 {
		st := stations[0]
		iconKey := strings.ToLower(stationKey(st.url))
		iconKey = strings.TrimSuffix(iconKey, ".mp3")
		if iconKey == "nightride" {
			iconKey = "nrfm"
		}
		m.updateSelectorColors(iconKey)
	}

	return m
}

func fetchAllMetaCmd() tea.Cmd { return func() tea.Msg { return fetchAllMeta() } }

func (m model) Init() tea.Cmd {
	return tea.Batch(
		startStreamCmd(0, m.ampChan),
		fetchAllMetaCmd(),
		visualizerTick(),
		scrollTick(),
	)
}

const globalGain = 0.55

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "h":
			m.showHelp = !m.showHelp
			return m, nil
		case "l":
			m.isHorizontalLayout = !m.isHorizontalLayout
			return m, nil
		case "z":
			m.easterEgg = !m.easterEgg
			return m, nil
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
			m.startTime = time.Now()
			return m, startStreamCmd(idx, m.ampChan)
		case "up", "down", "k", "j":
			var cmd tea.Cmd
			m.l, cmd = m.l.Update(msg)
			m.listScrollOffset = 0
			idx := m.l.Index()
			if idx < len(stations) {
				st := stations[idx]
				iconKey := strings.ToLower(stationKey(st.url))
				iconKey = strings.TrimSuffix(iconKey, ".mp3")
				if iconKey == "nightride" {
					iconKey = "nrfm"
				}
				m.updateSelectorColors(iconKey)
			}
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
				m.originalTitles[i] = meta.title

				if i == m.playingIdx {
					iconKey := strings.ToLower(stationKey(st.url))
					iconKey = strings.TrimSuffix(iconKey, ".mp3")
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
						Timestamps: &client.Timestamps{Start: &m.startTime},
						Buttons: []*client.Button{
							{Label: "Listen to " + st.name, Url: "https://nightride.fm/?station=" + st.id()},
							{Label: "Join the Discord", Url: "https://discord.gg/synthwave"},
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
	case string:
		if msg == "visualizerTick" {
			select {
			case amps := <-m.ampChan:
				height := asciiArtHeight()

				frameMax := 0.0
				for _, a := range amps {
					if a > frameMax {
						frameMax = a
					}
				}
				const peakDecay = 0.93
				m.visPeak *= peakDecay
				if frameMax > m.visPeak {
					m.visPeak = frameMax
				}
				if m.visPeak < 1e-6 {
					m.visPeak = 1e-6
				}

				gain := globalGain * float64(height-1) / m.visPeak

				const (
					gamma = 0.85
					atk   = 0.9
					dec   = 0.30
				)

				for i := range m.barHeights {
					shaped := math.Pow(amps[i], gamma) * gain
					current := float64(m.barHeights[i])
					a := dec
					if shaped > current {
						a = atk
					}
					m.barHeights[i] = int(a*shaped + (1-a)*current)
				}

				prev := m.barHeights
				blur := make([]int, len(prev))
				for i := range prev {
					l := prev[max(i-1, 0)]
					m2 := prev[i]
					r := prev[min(i+1, len(prev)-1)]
					blur[i] = (l + m2*2 + r) / 4
				}
				m.barHeights = blur

			default:
			}
			return m, visualizerTick()
		}
		if msg == "scrollTick" {
			if m.playingIdx != -1 {
				originalTitle := m.originalTitles[m.playingIdx]
				if len(originalTitle) > 43 {
					m.scrollOffset += m.scrollStep
				}
			}

			hoveredIdx := m.l.Index()
			if hoveredIdx >= 0 && hoveredIdx < len(stations) && hoveredIdx != m.playingIdx {
				originalTitle := m.originalTitles[hoveredIdx]
				if len(originalTitle) > 43 {
					m.listScrollOffset += m.scrollStep
					displayTitle := originalTitle
					paddedTitle := displayTitle + "    "
					totalLen := len(paddedTitle)
					scrollPos := m.listScrollOffset % totalLen

					if scrollPos+43 <= totalLen {
						displayTitle = paddedTitle[scrollPos : scrollPos+43]
					} else {
						part1 := paddedTitle[scrollPos:]
						part2 := paddedTitle[:43-len(part1)]
						displayTitle = part1 + part2
					}

					hoveredStation := stations[hoveredIdx]
					hoveredStation.title = displayTitle
					m.l.SetItem(hoveredIdx, hoveredStation)
				}
			}

			return m, scrollTick()
		}
	}

	var cmd tea.Cmd
	m.l, cmd = m.l.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.easterEgg {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff386f")).
			Render(EasterEgg)
	}
	header := "  ▐▐ PAUSED\n"
	currentIconKey := "nrfm"

	if m.playingIdx != -1 {
		item := m.l.Items()[m.playingIdx].(station)
		currentIconKey = strings.ToLower(stationKey(item.url))
		currentIconKey = strings.TrimSuffix(currentIconKey, ".mp3")
		if currentIconKey == "nightride" {
			currentIconKey = "nrfm"
		}

		originalTitle := m.originalTitles[m.playingIdx]
		displayTitle := originalTitle
		maxWidth := 43
		if len(originalTitle) > maxWidth {
			padded := originalTitle + "    "
			total := len(padded)
			pos := m.scrollOffset % total
			if pos+maxWidth <= total {
				displayTitle = padded[pos : pos+maxWidth]
			} else {
				part1 := padded[pos:]
				part2 := padded[:maxWidth-len(part1)]
				displayTitle = part1 + part2
			}
		}

		header = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).
			Render("  " + item.name + "\n  " + "▶ " + displayTitle)
	}

	var visual string
	if m.showHelp {
		controlsText := `
┌───────────── HELP ─────────────┐
│ Controls:                      │
│ ↑/↓, j/k    Navigate stations  │
│ Enter↵      Play/Pause         │
│ L           Toggle layout      │
│ H           Show/hide help     │
│ Z           Easter egg mode    │
│ Q, Ctrl+C   Quit               │
└────────────────────────────────┘
 Press [H] again to hide help...

MORE INFO:
· Nightride FM
https://nightride.fm
· GitHub
https://github.com/babycommando/nightride-cli
`
		asciiWidth := asciiArtWidth()
		visual = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff386f")).
			Width(asciiWidth).
			Render(controlsText)
	} else {
		visual = RenderVisualizedASCII(m.barHeights, currentIconKey)
	}

	if m.isHorizontalLayout {
		rightPanel := lipgloss.JoinVertical(lipgloss.Left, header, m.l.View())
		rightPanelWithMargin := lipgloss.NewStyle().MarginTop(1).Render(rightPanel)
		return lipgloss.JoinHorizontal(lipgloss.Top, visual, rightPanelWithMargin)
	}

	visualWithMargin := lipgloss.NewStyle().MarginLeft(1).Render(visual)
	return lipgloss.JoinVertical(lipgloss.Left, visualWithMargin, header, m.l.View())
}

/* audio */

var (
	speakerOnce     sync.Once
	mixerSampleRate beep.SampleRate
)

func (m *model) stopCurrent() {
	_ = client.SetActivity(client.Activity{})
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

func dialAndDecode(url string, tries int) (beep.StreamSeekCloser, beep.Format, io.ReadCloser, error) {
	for i := 0; i < tries; i++ {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}

		decoded, format, err := mp3.Decode(resp.Body)
		if err == nil {
			return decoded, format, resp.Body, nil
		}

		resp.Body.Close()
		time.Sleep(250 * time.Millisecond)
	}
	return nil, beep.Format{}, nil, fmt.Errorf("failed to decode")
}

func startStreamCmd(idx int, ampChan chan []float64) tea.Cmd {
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

		vs := &visualizerStreamer{
			Streamer: beep.Streamer(decoded),
			ampChan:  ampChan,
			width:    asciiArtWidth(),
		}

		playStream := beep.Streamer(vs)
		if format.SampleRate != mixerSampleRate {
			playStream = beep.Resample(4, format.SampleRate, mixerSampleRate, vs)
		}

		speaker.Clear()
		speaker.Play(playStream)

		iconKey := strings.ToLower(stationKey(st.url))
		iconKey = strings.TrimSuffix(iconKey, ".mp3")
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

		now := time.Now()
		_ = client.SetActivity(client.Activity{
			Type:       2,
			State:      artist,
			Details:    "Listening to " + st.name,
			LargeImage: iconKey,
			SmallImage: "nrfm",
			LargeText:  track,
			Timestamps: &client.Timestamps{Start: &now},
			Buttons: []*client.Button{
				{Label: "Listen to " + st.name, Url: "https://nightride.fm/?station=" + st.id()},
				{Label: "Join the Discord", Url: "https://discord.gg/synthwave"},
			},
		})

		return streamHandleMsg{streamer: decoded, body: body}
	}
}

/* meta */

type nowPlaying struct {
	Station string `json:"station"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
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
				title:     fmt.Sprintf("%s – %s", np.Artist, np.Title),
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
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return fetchAllMeta() })
}

func visualizerTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg { return "visualizerTick" })
}

func scrollTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return "scrollTick" })
}

type visualizerStreamer struct {
	beep.Streamer
	ampChan chan []float64
	width   int
}

func (vs *visualizerStreamer) Stream(samples [][2]float64) (int, bool) {
	n, ok := vs.Streamer.Stream(samples)

	cols := vs.width
	amps := make([]float64, cols)

	bucket := n / cols
	if bucket == 0 {
		bucket = 1
	}

	for c := 0; c < cols; c++ {
		start := c * bucket
		end := start + bucket
		if end > n {
			end = n
		}

		var sumSq float64
		for i := start; i < end; i++ {
			s := (samples[i][0] + samples[i][1]) * 0.5
			sumSq += s * s
		}
		rms := math.Sqrt(sumSq / float64(end-start))
		amps[c] = rms
	}

	select {
	case vs.ampChan <- amps:
	default:
	}
	return n, ok
}

/* ───────────── ROOT WRAPPER (player + zuse) ───────────── */

type rootModel struct {
	active int // 0 = player, 1 = irc
	player model
	irc    *ZuseModel
}

func newRootModel() rootModel {
	return rootModel{
		active: 0,
		player: newModel(),
		irc:    NewZuseModel(),
	}
}

func (r rootModel) Init() tea.Cmd {
	return tea.Batch(r.player.Init(), r.irc.Init())
}

func (r rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// handle tab switching here
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "tab" {
		if r.active == 0 {
			r.active = 1
		} else {
			r.active = 0
		}
		return r, nil
	}

	// Window size must go to both so they layout correctly
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)

		iNew, iCmd := r.irc.Update(msg)
		r.irc = iNew.(*ZuseModel)

		return r, tea.Batch(pCmd, iCmd)
	}

	// Key events (except tab) only to active view
	if _, ok := msg.(tea.KeyMsg); ok {
		if r.active == 0 {
			pNew, pCmd := r.player.Update(msg)
			r.player = pNew.(model)
			return r, pCmd
		}
		iNew, iCmd := r.irc.Update(msg)
		r.irc = iNew.(*ZuseModel)
		return r, iCmd
	}

	// Non-key events: let the player always handle (audio/meta), and irc will ignore most.
	pNew, pCmd := r.player.Update(msg)
	r.player = pNew.(model)

	if r.active == 1 {
		iNew, iCmd := r.irc.Update(msg)
		r.irc = iNew.(*ZuseModel)
		return r, tea.Batch(pCmd, iCmd)
	}
	return r, pCmd
}

func (r rootModel) View() string {
	if r.active == 0 {
		return r.player.View()
	}
	return r.irc.View()
}

/* ───────────── main ───────────── */

func main() {
	os.Args[0] = "Nightride + ZUSE"

	if err := client.Login("1396017162425991279"); err != nil {
		fmt.Println("discord rpc error:", err)
	}

	if err := tea.NewProgram(newRootModel(), tea.WithAltScreen()).Start(); err != nil && err != io.EOF {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
}
