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

/* ─────────────  Bubble Tea model  ───────────── */

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
	ampChan            chan []float64 // amplitude data for visualizer
	easterEgg          bool
	visPeak            float64
	isHorizontalLayout bool           // toggle field for layout
	showHelp           bool           // toggle field for Help display
	scrollOffset       int            // Song title text scrolling
	listScrollOffset   int            // List item text scrolling
	originalTitles     map[int]string // Store original titles to prevent data corruption
	scrollStep         int            // Add this field to control scroll increment
}

// updateSelectorColors updates the list delegate colors based on the station's color scheme
func (m *model) updateSelectorColors(iconKey string) {
	colors, exists := StationColors[iconKey]
	if !exists {
		// Default to the original pink-purple gradient
		colors = []string{"#ff386f", "#7d3cff"}
	}

	// Create a new delegate with updated colors
	delegate := list.NewDefaultDelegate()

	// Set foreground (text) and bar (left border) color
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Copy().
		Foreground(lipgloss.Color(colors[0])).
		BorderLeftForeground(lipgloss.Color(colors[0]))

	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Copy().
		Foreground(lipgloss.Color(colors[1])).
		BorderLeftForeground(lipgloss.Color(colors[1]))

	// Update the list with the new delegate
	m.l.SetDelegate(delegate)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

/* ─────────────  Bubble Tea setup  ───────────── */
func newModel() model {
	items := make([]list.Item, len(stations))
	for i := range stations {
		items[i] = stations[i]
	}
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 46, len(items)*2-3)
	l.Title = "↑/↓ Navigate · [Enter↵] Play/Pause · [H]elp"
	// Hide Bubble Tea list component
	l.Styles.Title = l.Styles.Title.Copy(). // Need to maintain this even if not used for the background color removal!!!
						UnsetBackground().                     //	 Remove background color
						MarginTop(2).                          // Add top margin
						Margin(0, 0).                          // Remove side margins
						Padding(0, 0).                         // Remove padding
						Foreground(lipgloss.Color("#CCCCCC")). // Changes the color for the l.Title bar
						Faint(true)                            // This sets opacity to approximately 75%
	l.SetShowStatusBar(false)    // Hide the status bar
	l.SetFilteringEnabled(false) // Disable filtering
	l.SetShowHelp(false)         // Add this line to hide the help text

	// Initialize originalTitles map
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
		isHorizontalLayout: true,           // Set default to horizontal layout
		showHelp:           false,          // Add this field
		originalTitles:     originalTitles, // Store original titles
		scrollStep:         1,              // Add smooth scrolling step
	}

	// Set initial selector colors for the first station
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
			m.showHelp = !m.showHelp // toggle help display
			return m, nil
		case "l":
			m.isHorizontalLayout = !m.isHorizontalLayout // toggle layout
			return m, nil
		case "z":
			m.easterEgg = !m.easterEgg // toggle easter egg mode
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
			// Reset list scroll when changing selection for smoother transition
			m.listScrollOffset = 0
			// Update selector colors based on the currently hovered station
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
				// Update original titles map
				m.originalTitles[i] = meta.title

				// 🔁 Update Discord status *only for current station*
				if i == m.playingIdx {
					// now := time.Now()
					iconKey := strings.ToLower(stationKey(st.url)) // "Darksynth.mp3" → "darksynth.mp3"
					iconKey = strings.TrimSuffix(iconKey, ".mp3")  // → "darksynth"
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
	case string:
		if msg == "visualizerTick" {
			select {
			case amps := <-m.ampChan:
				height := asciiArtHeight()

				// ── stable gain via slow‑decaying peak envelope ──
				frameMax := 0.0
				for _, a := range amps {
					if a > frameMax {
						frameMax = a
					}
				}
				const peakDecay = 0.93 // slower → smoother gain
				m.visPeak *= peakDecay
				if frameMax > m.visPeak {
					m.visPeak = frameMax
				}
				if m.visPeak < 1e-6 {
					m.visPeak = 1e-6
				}

				gain := globalGain * float64(height-1) / m.visPeak

				// ── column processing ──
				const (
					gamma = 0.85 // closer to raw RMS, punchier transients
					atk   = 0.9  // bars shoot up almost instantly
					dec   = 0.30 // bars fall a bit quicker
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

				// ── light 3‑tap blur ──
				prev := m.barHeights
				blur := make([]int, len(prev))
				for i := range prev {
					l := prev[max(i-1, 0)]
					m2 := prev[i]
					r := prev[min(i+1, len(prev)-1)]
					blur[i] = (l + m2*2 + r) / 4
				}
				m.barHeights = blur

			default: /* no new amplitudes */
			}
			return m, visualizerTick()
		}
		if msg == "scrollTick" {
			// Only scroll if there's a playing station with long title
			if m.playingIdx != -1 {
				// Use original title for comparison and scrolling
				originalTitle := m.originalTitles[m.playingIdx]
				if len(originalTitle) > 43 {
					m.scrollOffset += m.scrollStep
				}
			}

			// Scroll the currently hovered list item (but not if it's the playing station)
			hoveredIdx := m.l.Index()
			if hoveredIdx >= 0 && hoveredIdx < len(stations) && hoveredIdx != m.playingIdx {
				// Use original title for comparison
				originalTitle := m.originalTitles[hoveredIdx]
				if len(originalTitle) > 43 { // Adjust max width as needed
					m.listScrollOffset += m.scrollStep
					// Create scrolled display title from original
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

					// Update only the list item display, not the original data
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

	header := "▷ Paused\n  Press [Enter ↵] to Play/Pause"
	var currentIconKey string = "nrfm" // default for paused state

	if m.playingIdx != -1 {
		item := m.l.Items()[m.playingIdx].(station)

		// Get the icon key for the current station
		currentIconKey = strings.ToLower(stationKey(item.url))
		currentIconKey = strings.TrimSuffix(currentIconKey, ".mp3")
		if currentIconKey == "nightride" {
			currentIconKey = "nrfm"
		}

		// Handle scrolling text for long titles using original title
		originalTitle := m.originalTitles[m.playingIdx]
		displayTitle := originalTitle
		maxWidth := 43
		if len(originalTitle) > maxWidth {
			// Add padding to create smooth scrolling
			paddedTitle := originalTitle + "    " // Add some spaces between loops
			totalLen := len(paddedTitle)

			// Calculate scroll position
			scrollPos := m.scrollOffset % totalLen

			// Create the visible portion
			if scrollPos+maxWidth <= totalLen {
				displayTitle = paddedTitle[scrollPos : scrollPos+maxWidth]
			} else {
				// Handle wrap-around
				part1 := paddedTitle[scrollPos:]
				part2 := paddedTitle[:maxWidth-len(part1)]
				displayTitle = part1 + part2
			}
		}

		header = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).
			Render("  " + item.name + "\n  " + "▶ " + displayTitle)
	}

	var visual string
	if m.showHelp {
		// Show controls in place of ASCII art, maintaining the same width
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

		// Get the width of the ASCII art to maintain consistent layout
		asciiWidth := asciiArtWidth()
		visual = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff386f")).
			Width(asciiWidth).
			Render(controlsText)
	} else {
		visual = RenderVisualizedASCII(m.barHeights, currentIconKey)
	}

	if m.isHorizontalLayout {
		// Horizontal layout: ASCII art (or controls) on left, station list on right
		rightPanel := lipgloss.JoinVertical(lipgloss.Left, header, m.l.View())
		rightPanelWithMargin := lipgloss.NewStyle().MarginTop(1).Render(rightPanel)
		return lipgloss.JoinHorizontal(lipgloss.Top, visual, rightPanelWithMargin)
	} else {
		// Vertical layout: everything stacked vertically (original layout)
		// ASCII art (or controls) on top, station list below
		visualWithMargin := lipgloss.NewStyle().MarginLeft(1).Render(visual)
		return lipgloss.JoinVertical(lipgloss.Left, visualWithMargin, header, m.l.View())
	}
}

// ─────────────  global audio state  ─────────────
var (
	speakerOnce     sync.Once
	mixerSampleRate beep.SampleRate
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
	format beep.Format,
	body io.ReadCloser,
	err error,
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
				listeners int //	not used
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
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return fetchAllMeta()
	})
}

// Visualizer tick for updating the visualizer bars
func visualizerTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg {
		return "visualizerTick"
	})
}

// Scroll speed tick for song title
func scrollTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return "scrollTick"
	})
}

/* ─────────────  main  ───────────── */

func main() {
	// Set the process title
	os.Args[0] = "Nightride FM"

	err := client.Login("1396017162425991279")
	if err != nil {
		fmt.Println("discord rpc error:", err)
	}

	if err := tea.NewProgram(newModel(), tea.WithAltScreen()).Start(); err != nil && err != io.EOF {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
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

	// bucket == how many samples belong to ONE bar
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
			s := (samples[i][0] + samples[i][1]) * 0.5 // mono
			sumSq += s * s                             // power
		}
		rms := math.Sqrt(sumSq / float64(end-start)) // 0 … 1
		amps[c] = rms
	}

	select {
	case vs.ampChan <- amps:
	default:
	}
	return n, ok
}
