package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"golang.org/x/term"
)

type Key int

type Style struct {
	fg   int
	bg   int
	bold bool
}

var initialState *term.State

const KEY_CHANNEL_SIZE = 32

const (
	KeyUp    Key = 256
	KeyDown  Key = 257
	KeyLeft  Key = 258
	KeyRight Key = 259
	KeyCtrlC Key = 3
	KeyCtrlR Key = 18
	KeyEnter Key = '\r'
)

const (
	Black         int = 30
	Red           int = 31
	Green         int = 32
	Yellow        int = 33
	Blue          int = 34
	Magenta       int = 35
	Cyan          int = 36
	White         int = 37
	Default       int = 39
	Gray          int = 90
	BrightRed     int = 91
	BrightGreen   int = 92
	BrightYellow  int = 93
	BrightBlue    int = 94
	BrightMagenta int = 95
	BrightCyan    int = 96
	BrightWhite   int = 97
)

// ClearTerminal clears the terminal.
func ClearTerminal() {
	fmt.Print("\x1b[2J")
}

// FiniTerminal restores the terminal to its normal state.
func FiniTerminal() {
	// Disable invisible cursor and alternative terminal buffer.
	fmt.Print("\x1b[?25h\x1b[?1049l")
	term.Restore(int(os.Stdin.Fd()), initialState)
}

// GetSize returns the terminal size.
func GetSize() (int, int, error) {
	return term.GetSize(int(os.Stdin.Fd()))
}

// InitTerminal initializes the terminal to display any UI components.
func InitTerminal() error {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	initialState = state

	// Enable invisible cursor and alternative terminal buffer.
	fmt.Print("\x1b[?25l\x1b[?1049h")
	return nil
}

// Listen returns a channel of keypress events from the terminal. The channel
// will be closed either when the context is cancelled or an error occurs.
func Listen(ctx context.Context) <-chan Key {
	ch := make(chan Key, KEY_CHANNEL_SIZE)
	reader := bufio.NewReader(os.Stdin)
	buf := make([]byte, 32)
	go func() {
		defer close(ch)
		for {
			// Check if the listener should be cancelled.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read the most recent bytes from stdin to buf.
			n, err := reader.Read(buf)
			if err != nil {
				return
			}

			// If more than 3 bytes were read, it's not something we need.
			if n > 3 {
				continue
			}
			switch n {
			case 1:
				ch <- Key(buf[0])
				continue
			case 3:
				switch string(buf[:3]) {
				case "\x1b[A":
					ch <- KeyUp
				case "\x1b[B":
					ch <- KeyDown
				case "\x1b[C":
					ch <- KeyRight
				case "\x1b[D":
					ch <- KeyLeft
				default:
					continue
				}
			}
		}
	}()
	return ch
}

// NewStyle returns a new style with the default colors.
func NewStyle() Style {
	return Style{
		fg: 49,
		bg: 49,
	}
}

func (s Style) Bold() Style {
	s.bold = true
	return s
}

func (s Style) Background(c int) Style {
	s.bg = c + 10
	return s
}

func (s Style) Foreground(c int) Style {
	s.fg = c
	return s
}

func (s Style) Render(in string) {
	txt := fmt.Sprintf("\x1b[0;%d;%d", s.fg, s.bg)
	if s.bold {
		txt += ";1"
	}
	txt += "m"
	fmt.Print(txt, in)
}

func (s Style) RenderAt(in string, x, y int) {
	s.Render(fmt.Sprintf("\x1b[%d;%dH", y, x) + in)
}
