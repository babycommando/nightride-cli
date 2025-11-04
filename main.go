package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

/* ───────────── logs monitor ───────────── */

type ringLogger struct {
	mu  sync.Mutex
	buf []string
	cap int
}

func setTitleNightride() { fmt.Print("\x1b]0;Nightride\x07") }

func newRingLogger(capacity int) *ringLogger { return &ringLogger{cap: capacity} }

func (r *ringLogger) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := time.Now().Format("15:04:05 ")
	if len(r.buf) == r.cap {
		copy(r.buf, r.buf[1:])
		r.buf[len(r.buf)-1] = ts + s
		return
	}
	r.buf = append(r.buf, ts+s)
}

func (r *ringLogger) last(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.buf) {
		n = len(r.buf)
	}
	out := make([]string, n)
	copy(out, r.buf[len(r.buf)-n:])
	return out
}

var logmem = newRingLogger(2000)

func logf(format string, a ...any) { logmem.add(fmt.Sprintf(format, a...)) }

var origStderr = os.Stderr

func redirectStderrToMonitor() func() {
	r, w, _ := os.Pipe()
	os.Stderr = w

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(r)
		buf := make([]byte, 64*1024)
		sc.Buffer(buf, 1024*1024)
		for sc.Scan() {
			logmem.add(sc.Text())
		}
	}()

	return func() {
		_ = w.Close()
		<-done
		os.Stderr = origStderr
	}
}

/* ─────────────  Bubble Tea model (player)  ───────────── */

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
	showMonitor        bool
}

type fadeIn struct {
	s     beep.Streamer
	total int
	done  int
}

func newFadeIn(s beep.Streamer, sr beep.SampleRate, d time.Duration) *fadeIn {
	n := sr.N(d)
	if n < 1 {
		n = 1
	}
	return &fadeIn{s: s, total: n}
}

func (f *fadeIn) Stream(samples [][2]float64) (int, bool) {
	n, ok := f.s.Stream(samples)
	if f.done >= f.total {
		return n, ok
	}
	remain := f.total - f.done
	limit := n
	if limit > remain {
		limit = remain
	}
	start := f.done
	for i := 0; i < limit; i++ {
		g := float64(start+i+1) / float64(f.total)
		samples[i][0] *= g
		samples[i][1] *= g
	}
	f.done += limit
	if f.done > f.total {
		f.done = f.total
	}
	return n, ok
}

func (f *fadeIn) Err() error {
	if e, ok := f.s.(interface{ Err() error }); ok {
		return e.Err()
	}
	return nil
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
	l.Title = "↑/↓ Navigate · [↵] Play/Pause · [H] Help"
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

func (m model) Init() tea.Cmd {
	return tea.Batch(
		startStreamCmd(0, m.ampChan),
		startSSECmd(),
		waitMetaCmd(),
		visualizerTick(),
		scrollTick(),
	)
}

const globalGain = 0.55

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "m":
			m.showMonitor = !m.showMonitor
			return m, nil
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

					parts := strings.SplitN(st.title, " - ", 2)
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
						logf("discord rpc: %v", err)
					}
				}
			}
		}
		return m, waitMetaCmd()
	case errMsg:
		logf("audio error: %v", msg)
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
	if m.showMonitor {
		lines := logmem.last(400)
		var b bytes.Buffer
		for _, s := range lines {
			b.WriteString(s)
			b.WriteByte('\n')
		}
		title := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).Render("Monitor [M to exit]")
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1, 2).Width(asciiArtWidth() + 56)
		return box.Render(title + "\n" + b.String())
	}

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
┌──────────── HELP ────────────┐
│ ↑/↓, j/k  Navigate stations  │
│ Enter     Play/Pause         │
│ L         Toggle layout      │
│ H         Show/hide help     │
│ D         Toggle Doom        │
│ M         Monitor logs       │
│ Y         YouTube ASCII      │
│ Z         Easter egg         │
│ Q,Ctrl+C  Quit               │
└──────────────────────────────┘
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

func dialAndDecode(u string, tries int) (beep.StreamSeekCloser, beep.Format, io.ReadCloser, error) {
	for i := 0; i < tries; i++ {
		resp, err := http.Get(u)
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

		vs := &visualizerStreamer{Streamer: beep.Streamer(decoded), ampChan: ampChan, width: asciiArtWidth()}

		playStream := beep.Streamer(vs)
		if format.SampleRate != mixerSampleRate {
			playStream = beep.Resample(4, format.SampleRate, mixerSampleRate, vs)
		}
		fade := newFadeIn(playStream, mixerSampleRate, 650*time.Millisecond)

		speaker.Clear()
		speaker.Play(fade)

		iconKey := strings.ToLower(stationKey(st.url))
		iconKey = strings.TrimSuffix(iconKey, ".mp3")
		if iconKey == "nightride" {
			iconKey = "nrfm"
		}

		parts := strings.SplitN(st.title, " - ", 2)
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

/* meta via SSE */

type nowPlaying struct {
	Station string `json:"station"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
}

var (
	sseOnce sync.Once
	sseChan = make(chan metaAllMsg, 64)
)

func startSSECmd() tea.Cmd {
	return func() tea.Msg { sseOnce.Do(func() { go sseLoop() }); return nil }
}

func waitMetaCmd() tea.Cmd { return func() tea.Msg { return <-sseChan } }

func sseLoop() {
	clientHTTP := &http.Client{Timeout: 0}
	backoff := time.Second

	for {
		req, _ := http.NewRequest("GET", "https://nightride.fm/meta", nil)
		req = req.WithContext(context.Background())
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := clientHTTP.Do(req)
		if err != nil {
			logf("meta connect: %v", err)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			logf("meta status: %s", resp.Status)
			resp.Body.Close()
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		backoff = time.Second
		reader := bufio.NewReader(resp.Body)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				logf("meta read: %v", err)
				resp.Body.Close()
				break
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "" || payload == "keepalive" {
				continue
			}

			var entries []nowPlaying
			if err := json.Unmarshal([]byte(payload), &entries); err != nil {
				continue
			}

			update := metaAllMsg{}
			for _, np := range entries {
				update[np.Station+".mp3"] = struct {
					title     string
					listeners int
				}{
					title:     fmt.Sprintf("%s - %s", np.Artist, np.Title),
					listeners: 0,
				}
			}

			select {
			case sseChan <- update:
			default:
			}
		}
	}
}

func visualizerTick() tea.Cmd { return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg { return "visualizerTick" }) }
func scrollTick() tea.Cmd     { return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return "scrollTick" }) }

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

/* ───────────── IN-APP YouTube ASCII SCREEN ───────────── */

const ytRamp = " .:-=+*#%@"

var asciiLUT [256]byte

func init() {
	r := []byte(ytRamp)
	n := len(r) - 1
	for i := 0; i < 256; i++ {
		asciiLUT[i] = r[(i*n)>>8]
	}
}

type ytFrameMsg struct {
	s       string
	session int
}
type ytStopMsg struct{ session int }
type ytErrMsg struct {
	err     error
	session int
}

type ytModel struct {
	active    bool
	idx       int
	cols      int
	rows      int
	frame     string
	hasVideo  bool
	loading   bool
	colorMode bool

	session int

	cmd    *exec.Cmd
	out    io.ReadCloser
	done   chan struct{}
	frames chan []byte
}

func (y *ytModel) View() string {
	if !y.active {
		return ""
	}
	if !y.hasVideo {
		msg := "This station has no video stream. ←/→ switch · [Q] back"
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#aaa")).
			Render(centerLine(msg, max(10, y.cols)))
	}
	if y.loading {
		msg := "Loading video…"
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#aaa")).
			Render(centerLine(msg, max(10, y.cols)))
	}

	stationName := ""
	meta := ""
	if y.idx >= 0 && y.idx < len(stations) {
		stationName = stations[y.idx].name
		meta = stations[y.idx].title
	}
	head := fmt.Sprintf("YouTube ASCII · %s", stationName)
	if meta != "" {
		head += " · " + meta
	}
	headStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).Render(head)

	mode := "color"
	if y.colorMode {
		mode = "no color"
	}
	controls := fmt.Sprintf("[←/→] station · [Ctrl -/+] resize · [C] %s · [Q] quit", mode)
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render(padOrTrim(controls, max(10, y.cols)))

	return headStyled + "\n" + y.frame + "\n" + bar
}

func (y *ytModel) start(idx, cols, rows int) tea.Cmd {
	y.stop()
	y.active = true
	y.idx = idx
	y.cols = cols
	if rows < 8 {
		rows = 8
	}
	y.rows = rows - 3
	y.frame = ""
	y.loading = true
	y.hasVideo = false
	y.session++

	st := stations[idx]
	if st.youtube == "" {
		y.hasVideo = false
		return nil
	}

	u, err := resolveYouTubeLiveHLS(st.youtube)
	if err != nil {
		y.hasVideo = false
		y.loading = false
		s := y.session
		return func() tea.Msg { return ytErrMsg{err: err, session: s} }
	}

	cmd, out, err := startFFMPEG(u, y.cols, y.rows)
	if err != nil {
		y.hasVideo = false
		y.loading = false
		s := y.session
		return func() tea.Msg { return ytErrMsg{err: err, session: s} }
	}

	y.cmd = cmd
	y.out = out
	y.done = make(chan struct{})
	y.frames = make(chan []byte, 2)
	y.hasVideo = true

	cur := y.session

	go func(sess int) {
		_ = cmd.Wait()
		close(y.done)
		if app != nil {
			app.Send(ytStopMsg{session: sess})
		}
	}(cur)

	go readFrames(out, y.cols*y.rows*3, y.frames)

	go func(col, row, sess int, ch <-chan []byte) {
		for f := range ch {
			var s string
			if y.colorMode {
				s = fastColorASCII(f, col, row)
			} else {
				s = fastMonoASCII(f, col, row)
			}
			if app != nil {
				app.Send(ytFrameMsg{s: s, session: sess})
			}
		}
	}(y.cols, y.rows, cur, y.frames)

	return nil
}

func (y *ytModel) stop() {
	if y.cmd != nil && y.cmd.Process != nil {
		_ = y.cmd.Process.Kill()
	}
	if y.out != nil {
		_ = y.out.Close()
	}
	if y.done != nil {
		select {
		case <-y.done:
		default:
		}
	}
	y.cmd, y.out, y.done, y.frames = nil, nil, nil, nil
}

func (y *ytModel) restartForSize(cols, rows int) tea.Cmd {
	if !y.active {
		return nil
	}
	return y.start(y.idx, cols, rows)
}

func (y *ytModel) nextIndex(delta int) int {
	n := len(stations)
	if n == 0 {
		return 0
	}
	i := y.idx + delta
	for i < 0 {
		i += n
	}
	return i % n
}

/* extremely fast color ASCII conversion */
func fastColorASCII(rgb []byte, cols, rows int) string {
	buf := make([]byte, 0, cols*rows*24)
	i := 0
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r := rgb[i]
			g := rgb[i+1]
			b := rgb[i+2]
			i += 3
			y8 := (54*int(r) + 183*int(g) + 19*int(b)) >> 8
			ch := asciiLUT[y8]
			buf = append(buf, '\x1b', '[', '3', '8', ';', '2', ';')
			buf = strconv.AppendInt(buf, int64(r), 10)
			buf = append(buf, ';')
			buf = strconv.AppendInt(buf, int64(g), 10)
			buf = append(buf, ';')
			buf = strconv.AppendInt(buf, int64(b), 10)
			buf = append(buf, 'm', ch)
		}
		buf = append(buf, '\x1b', '[', '0', 'm', '\n')
	}
	return string(buf)
}

/* ultra-fast mono version (no ANSI) */
func fastMonoASCII(rgb []byte, cols, rows int) string {
	buf := make([]byte, 0, cols*(rows+1))
	i := 0
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r := int(rgb[i])
			g := int(rgb[i+1])
			b := int(rgb[i+2])
			i += 3
			y8 := (54*r + 183*g + 19*b) >> 8
			buf = append(buf, asciiLUT[y8])
		}
		buf = append(buf, '\n')
	}
	return string(buf)
}

func padOrTrim(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if len(s) >= w {
		return s[:w]
	}
	return s + strings.Repeat(" ", w-len(s))
}

func centerLine(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	left := (w - len(s)) / 2
	return strings.Repeat(" ", left) + s
}

/* ───────────── ROOT WRAPPER (player + zuse + doom + yt-screen) ───────────── */

type doomExitMsg struct{ err error }

type rootModel struct {
	active int
	player model
	irc    *ZuseModel

	doomRunning bool

	yt ytModel

	termW int
	termH int
}

func newRootModel() rootModel {
	return rootModel{
		active:      0,
		player:      newModel(),
		irc:         NewZuseModel(),
		doomRunning: false,
	}
}

func (r rootModel) Init() tea.Cmd { return tea.Batch(r.player.Init(), r.irc.Init()) }

func (r *rootModel) switchAudioTo(i int) tea.Cmd {
	if i < 0 || i >= len(stations) {
		return nil
	}
	r.player.stopCurrent()
	r.player.playingIdx = i
	r.player.startTime = time.Now()
	return startStreamCmd(i, r.player.ampChan)
}

func (r rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case string:
		if m == "visualizerTick" || m == "scrollTick" {
			pNew, pCmd := r.player.Update(msg)
			r.player = pNew.(model)
			return r, pCmd
		}
	case metaAllMsg:
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)
		return r, pCmd
	case streamHandleMsg:
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)
		return r, pCmd
	case errMsg:
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)
		return r, pCmd

	case tea.WindowSizeMsg:
		r.termW, r.termH = m.Width, m.Height
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)
		iNew, iCmd := r.irc.Update(msg)
		r.irc = iNew.(*ZuseModel)
		if r.yt.active {
			return r, tea.Batch(pCmd, iCmd, r.yt.restartForSize(r.termW, r.termH))
		}
		return r, tea.Batch(pCmd, iCmd)

	case tea.KeyMsg:
		k := m.String()

		if r.yt.active {
			switch k {
			case "c", "C":
				r.yt.colorMode = !r.yt.colorMode
				r.yt.loading = true
				return r, r.yt.restartForSize(r.termW, r.termH)
			case "q", "Q":
				r.yt.stop()
				r.yt.active = false
				return r, tea.ClearScreen
			case "left":
				newIdx := r.yt.nextIndex(-1)
				vCmd := r.yt.start(newIdx, max(10, r.termW), max(12, r.termH))
				aCmd := r.switchAudioTo(newIdx)
				return r, tea.Batch(vCmd, aCmd)
			case "right":
				newIdx := r.yt.nextIndex(+1)
				vCmd := r.yt.start(newIdx, max(10, r.termW), max(12, r.termH))
				aCmd := r.switchAudioTo(newIdx)
				return r, tea.Batch(vCmd, aCmd)
			case "ctrl+-", "ctrl+_", "ctrl+=", "ctrl+plus", "ctrl+shift+=":
				return r, r.yt.restartForSize(r.termW, r.termH)
			default:
				return r, nil
			}
		}

		switch k {
		case "tab":
			if r.active == 0 {
				r.active = 1
			} else {
				r.active = 0
			}
			return r, nil
		case "d", "D":
				// ignore when IRC is active so you can type 'd'
				if r.active == 1 {
						break
				}
				if !r.doomRunning {
						return r, r.startDoom()
				}
				return r, nil
		case "y", "Y":
			if r.active == 1 {
				break
			}
			startIdx := r.player.playingIdx
			if startIdx < 0 || startIdx >= len(stations) {
				startIdx = 0
			}
			r.yt.active = true
			return r, r.yt.start(startIdx, max(10, r.termW), max(12, r.termH))
		}

		if r.active == 0 {
			pNew, pCmd := r.player.Update(msg)
			r.player = pNew.(model)
			return r, pCmd
		}
		iNew, iCmd := r.irc.Update(msg)
		r.irc = iNew.(*ZuseModel)
		return r, iCmd

	case ytFrameMsg:
		if m.session == r.yt.session {
			r.yt.frame = m.s
			r.yt.loading = false
			r.yt.hasVideo = true
		}
		return r, nil

	case ytStopMsg:
		if m.session == r.yt.session {
			r.yt.hasVideo = false
			r.yt.loading = false
		}
		return r, nil

	case ytErrMsg:
		if m.session == r.yt.session {
			logf("youtube ascii: %v", m.err)
			r.yt.hasVideo = false
			r.yt.loading = false
		}
		return r, nil

	case doomExitMsg:
		r.doomRunning = false
		if m.err != nil {
			logf("doom exit: %v", m.err)
		}
		return r, tea.ClearScreen
	}

	if r.active == 0 {
		pNew, pCmd := r.player.Update(msg)
		r.player = pNew.(model)
		return r, pCmd
	}
	iNew, iCmd := r.irc.Update(msg)
	r.irc = iNew.(*ZuseModel)
	return r, iCmd
}


func (r rootModel) View() string {
	if r.yt.active {
		stationName := ""
		meta := ""
		if r.yt.idx >= 0 && r.yt.idx < len(stations) {
			stationName = stations[r.yt.idx].name
			meta = stations[r.yt.idx].title
		}
		head := "  YouTube ASCII · " + stationName
		if meta != "" {
			head += " · " + meta
		}
		headStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD75F")).Render(head)
		return headStyled + "\n" + r.yt.View()
	}
	if r.active == 0 {
		return r.player.View()
	}
	return r.irc.View()
}

func (r *rootModel) startDoom() tea.Cmd {
	r.doomRunning = true
	return func() tea.Msg {
		if app != nil {
			if err := app.ReleaseTerminal(); err != nil {
				logf("release terminal: %v", err)
			}
		}
		exe, _ := os.Executable()
		var childDir string
		if _, err := os.Stat("doom1.wad"); err == nil {
			childDir = ""
		} else if exe != "" {
			d := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(d, "doom1.wad")); err == nil {
				childDir = d
			}
		}
		cmd := exec.Command(exe, "--doom")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = origStderr
		if childDir != "" {
			cmd.Dir = childDir
		}
		runErr := cmd.Run()
		if app != nil {
			if err := app.RestoreTerminal(); err != nil {
				logf("restore terminal: %v", err)
			}
		}
		return doomExitMsg{err: runErr}
	}
}

/* YouTube HLS resolver */

type ytPlayerResp struct {
	StreamingData struct {
		HLSManifestURL string `json:"hlsManifestUrl"`
	} `json:"streamingData"`
	PlayabilityStatus struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"playabilityStatus"`
}

func resolveYouTubeLiveHLS(pageURL string) (string, error) {
	id := extractYouTubeID(pageURL)
	if id == "" {
		return "", fmt.Errorf("bad YouTube URL or id")
	}

	watch := "https://www.youtube.com/watch?v=" + id + "&hl=en"
	html, err := httpGetString(watch, map[string]string{
		"User-Agent":      "Mozilla/5.0",
		"Accept-Language": "en-US,en;q=0.9",
	})
	if err != nil {
		return "", err
	}

	if js := extractJSONBlock(html, "ytInitialPlayerResponse"); js != "" {
		var pr ytPlayerResp
		if json.Unmarshal([]byte(js), &pr) == nil {
			if u := pr.StreamingData.HLSManifestURL; u != "" {
				return u, nil
			}
		}
	}

	apiKey := findFirst(`"INNERTUBE_API_KEY":"([^"]+)"`, html)
	clientVer := findFirst(`"INNERTUBE_CLIENT_VERSION":"([^"]+)"`, html)
	stsStr := findFirst(`"STS":([0-9]+)`, html)
	if apiKey == "" || clientVer == "" {
		return "", fmt.Errorf("missing api key or client version in page")
	}
	var sts int
	if stsStr != "" {
		sts, _ = strconv.Atoi(stsStr)
	}

	body := map[string]any{
		"videoId": id,
		"context": map[string]any{
			"client": map[string]any{
				"clientName":       "WEB",
				"clientVersion":    clientVer,
				"hl":               "en",
				"gl":               "US",
				"utcOffsetMinutes": 0,
			},
		},
		"racyCheckOk":    true,
		"contentCheckOk": true,
	}
	if sts > 0 {
		body["playbackContext"] = map[string]any{
			"contentPlaybackContext": map[string]any{
				"signatureTimestamp": sts,
			},
		}
	}

	bin, _ := json.Marshal(body)
	playerURL := "https://www.youtube.com/youtubei/v1/player?key=" + apiKey
	resp, err := httpPostJSON(playerURL, bin, map[string]string{
		"User-Agent":      "Mozilla/5.0",
		"Accept-Language": "en-US,en;q=0.9",
		"Content-Type":    "application/json",
	})
	if err != nil {
		return "", err
	}
	var pr ytPlayerResp
	if err := json.Unmarshal(resp, &pr); err != nil {
		return "", err
	}
	if pr.StreamingData.HLSManifestURL == "" {
		if pr.PlayabilityStatus.Status != "OK" {
			return "", fmt.Errorf("player status: %s %s", pr.PlayabilityStatus.Status, pr.PlayabilityStatus.Reason)
		}
		return "", fmt.Errorf("no hls manifest url")
	}
	return pr.StreamingData.HLSManifestURL, nil
}

/* helpers */

func extractYouTubeID(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.Contains(u, "youtube.com") {
		if parsed, err := url.Parse(u); err == nil {
			if v := parsed.Query().Get("v"); v != "" {
				return v
			}
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	if strings.Contains(u, "youtu.be/") {
		i := strings.Index(u, "youtu.be/")
		if i >= 0 {
			rest := u[i+len("youtu.be/"):]
			return strings.Split(strings.Trim(rest, "/"), "?")[0]
		}
	}
	return u
}

func httpGetString(u string, headers map[string]string) (string, error) {
	req, _ := http.NewRequest("GET", u, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func httpPostJSON(u string, payload []byte, headers map[string]string) ([]byte, error) {
	req, _ := http.NewRequest("POST", u, bytes.NewReader(payload))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func findFirst(re, s string) string {
	rx := regexp.MustCompile(re)
	m := rx.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractJSONBlock(html, name string) string {
	i := strings.Index(html, name)
	if i < 0 {
		return ""
	}
	j := strings.Index(html[i:], "{")
	if j < 0 {
		return ""
	}
	j += i
	depth := 0
	for k := j; k < len(html); k++ {
		switch html[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return html[j : k+1]
			}
		}
	}
	return ""
}

/* ───────────── main ───────────── */

var app *tea.Program

func hasArg(x string) bool {
	for _, a := range os.Args[1:] {
		if a == x {
			return true
		}
	}
	return false
}

func main() {
	os.Args[0] = "Nightride Client"

	if hasArg("--doom") {
		if err := RunDoom(nil); err != nil {
			fmt.Fprintln(os.Stderr, "doom:", err)
			os.Exit(1)
		}
		return
	}

	setTitleNightride()

	if err := client.Login("1396017162425991279"); err != nil {
		logf("discord rpc login: %v", err)
	}

	restoreStderr := redirectStderrToMonitor()
	defer restoreStderr()

	app = tea.NewProgram(newRootModel(), tea.WithAltScreen())
	if err := app.Start(); err != nil && err != io.EOF {
		logf("fatal: %v", err)
		os.Exit(1)
	}
}

var _ = image.Rect
var _ = color.RGBA{}

/* ───────────── ffmpeg process helpers (shared) ───────────── */

func startFFMPEG(hls string, cols, asciiRows int) (*exec.Cmd, io.ReadCloser, error) {
	args := []string{
			"-hide_banner", "-loglevel", "error",
			"-fflags", "nobuffer",
			"-re",
			"-probesize", "32k",
			"-rw_timeout", "1500000",
			"-thread_queue_size", "512",
			"-i", hls,
			"-an",
			"-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear,fps=20", cols, asciiRows),
			"-pix_fmt", "rgb24",
			"-f", "rawvideo", "pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, out, nil
}

func readFrames(out io.Reader, frameSize int, frames chan []byte) {
	defer close(frames)
	buf := make([]byte, frameSize)
	for {
		_, err := io.ReadFull(out, buf)
		if err != nil {
			return
		}
		cp := make([]byte, frameSize)
		copy(cp, buf)
		select {
		case frames <- cp:
		default:
			select {
			case <-frames:
			default:
			}
			frames <- cp
		}
	}
}
