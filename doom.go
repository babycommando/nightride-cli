package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/AndreRenaud/gore"
	"github.com/nfnt/resize"
	"golang.org/x/term"
)

// Characters from dark to bright
const ramp = " .:-=+*#%@"
const iwadName = "doom1.wad"

var restoreTTY func()

type termDoom struct {
    keys            <-chan byte
    outstandingDown map[uint8]time.Time

    quitting bool
    qstep    int
    qlast    time.Time
}

func (t *termDoom) DrawFrame(img *image.RGBA) {
    w, h, err := term.GetSize(int(os.Stdout.Fd()))
    if err != nil || w < 20 || h < 10 {
        w, h = 80, 24
    }
    h--

    target := resize.Resize(uint(w), uint(h), img, resize.NearestNeighbor)

    var b bytes.Buffer
    b.WriteString("\x1b[H")
    rgba, _ := ensureRGBA(target)
    toASCII(&b, rgba)
    _, _ = os.Stdout.Write(b.Bytes())
}

func (t *termDoom) SetTitle(_ string) {
	fmt.Fprint(os.Stdout, "\x1b]0;Nightride\x07")
}

func (t *termDoom) GetEvent(ev *gore.DoomEvent) bool {
    const upDelay = 60 * time.Millisecond
    now := time.Now()
    for k, ts := range t.outstandingDown {
        if now.Sub(ts) >= upDelay {
            delete(t.outstandingDown, k)
            ev.Type = gore.Ev_keyup
            ev.Key = k
            return true
        }
    }

    // Fast quit
    if t.quitting {
        if time.Since(t.qlast) < 12*time.Millisecond {
            return false
        }
        t.qlast = time.Now()
        switch t.qstep {
        case 0:
            ev.Type, ev.Key = gore.Ev_keydown, gore.KEY_ESCAPE
        case 1, 2, 3, 4:
            ev.Type, ev.Key = gore.Ev_keydown, gore.KEY_DOWNARROW1
        case 5:
            ev.Type, ev.Key = gore.Ev_keydown, gore.KEY_ENTER
        case 6:
            ev.Type, ev.Key = gore.Ev_keydown, 'y'
        default:
            if restoreTTY != nil {
                restoreTTY()
            }
            os.Exit(0)
        }
        t.outstandingDown[ev.Key] = time.Now()
        t.qstep++
        return true
    }

    select {
    case b, ok := <-t.keys:
        if !ok {
            return false
        }

        if b == 'd' || b == 'D' || b == 0x03 || b == 0x1A { // D, Ctrl+C, Ctrl+Z
            t.quitting = true
            t.qstep = 0
            t.qlast = time.Now().Add(-time.Hour)
            return false
        }

        seq := []byte{b}
        if b == 0x1b { 
            select {
            case b2 := <-t.keys:
                seq = append(seq, b2)
                select {
                case b3 := <-t.keys:
                    seq = append(seq, b3)
                default:
                }
            default:
            }
        }
        if k, ok := mapKey(seq); ok {
            ev.Type = gore.Ev_keydown
            ev.Key = k
            t.outstandingDown[k] = now
            return true
        }
        return false
    default:
        return false
    }
}

func ensureRGBA(img image.Image) (*image.RGBA, bool) {
    if r, ok := img.(*image.RGBA); ok {
        return r, true
    }
    b := img.Bounds()
    r := image.NewRGBA(b)
    for y := b.Min.Y; y < b.Max.Y; y++ {
        for x := b.Min.X; x < b.Max.X; x++ {
            r.Set(x, y, img.At(x, y))
        }
    }
    return r, false
}

func toASCII(w io.Writer, img *image.RGBA) {
    b := img.Bounds()
    last := color.RGBA{}
    for y := b.Min.Y; y < b.Max.Y; y++ {
        for x := b.Min.X; x < b.Max.X; x++ {
            o := (y-b.Min.Y)*img.Stride + (x-b.Min.X)*4
            r := img.Pix[o+0]
            g := img.Pix[o+1]
            bl := img.Pix[o+2]
            l := int(r)*3 + int(g)*6 + int(bl)*1
            idx := (l * (len(ramp) - 1)) / (255 * 10)
            if idx < 0 { idx = 0 }
            if idx >= len(ramp) { idx = len(ramp) - 1 }
            ch := ramp[idx]
            if r != last.R || g != last.G || bl != last.B {
                fmt.Fprintf(w, "\x1b[38;2;%d;%d;%dm", r, g, bl)
                last = color.RGBA{r, g, bl, 255}
            }
            _, _ = w.Write([]byte{byte(ch)})
        }
        _, _ = w.Write([]byte("\x1b[0m\r\n"))
        last = color.RGBA{}
    }
}

func mapKey(seq []byte) (uint8, bool) {
    s := string(seq)
    switch s {
    case "\x1b[A":
        return gore.KEY_UPARROW1, true
    case "\x1b[B":
        return gore.KEY_DOWNARROW1, true
    case "\x1b[C":
        return gore.KEY_RIGHTARROW1, true
    case "\x1b[D":
        return gore.KEY_LEFTARROW1, true
    case " ", "\x1bOP":
        return gore.KEY_USE1, true
    case "\r", "\n":
        return gore.KEY_ENTER, true
    case "\x1b":
        return gore.KEY_ESCAPE, true
    case "\t":
        return gore.KEY_TAB, true
    case ",":
        return gore.KEY_FIRE1, true
    }
    if len(seq) == 1 {
        if seq[0] >= '0' && seq[0] <= '9' {
            return seq[0], true
        }
        if seq[0] == 'y' || seq[0] == 'n' || seq[0] == 'Y' || seq[0] == 'N' {
            return toLower(seq[0]), true
        }
    }
    return 0, false
}

func toLower(b byte) uint8 {
    if b >= 'A' && b <= 'Z' {
        return b - 'A' + 'a'
    }
    return b
}

func keyReader(r io.Reader) <-chan byte {
    ch := make(chan byte, 128)
    br := bufio.NewReader(r)
    go func() {
        defer close(ch)
        for {
            b, err := br.ReadByte()
            if err != nil {
                return
            }
            ch <- b
        }
    }()
    return ch
}

func ensureIWADInCWD() error {
    if st, err := os.Stat(iwadName); err == nil && !st.IsDir() {
        return nil
    }
    exe, err := os.Executable()
    if err != nil {
        return errors.New("doom1.wad not found and exe dir unknown")
    }
    dir := filepath.Dir(exe)
    if st, err := os.Stat(filepath.Join(dir, iwadName)); err == nil && !st.IsDir() {
        return os.Chdir(dir)
    }
    return errors.New("doom1.wad not found in cwd or exe dir")
}

// RunDoom runs Doom
func RunDoom(args []string) error {
    fd := int(os.Stdin.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        return fmt.Errorf("terminal raw mode: %w", err)
    }
    restoreTTY = func() {
        _ = term.Restore(fd, oldState)
        fmt.Print("\x1b[0m\x1b[2J\x1b[H\x1b[?25h")
    }
    defer func() {
        if restoreTTY != nil {
            restoreTTY()
        }
    }()

    fmt.Print("\x1b[2J\x1b[H\x1b[?25l")

    td := &termDoom{
        keys:            keyReader(os.Stdin),
        outstandingDown: make(map[uint8]time.Time),
    }

    if len(args) == 0 {
        if err := ensureIWADInCWD(); err != nil {
            return err
        }
        args = []string{"-iwad", iwadName}
    }

    gore.Run(td, args)
    return nil
}
