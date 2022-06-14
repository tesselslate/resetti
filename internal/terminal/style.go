package terminal

import "fmt"

const (
	Black   int = 30
	Red     int = 31
	Green   int = 32
	Yellow  int = 33
	Blue    int = 34
	Magenta int = 35
	Cyan    int = 36
	White   int = 37
	Default int = 39
)

type Style struct {
	fg   int
	bg   int
	bold bool
}

func NewStyle() Style {
	return Style{}
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
	txt := fmt.Sprintf("\x1b[%d;%d", s.fg, s.bg)
	if s.bold {
		txt += ";1"
	}
	fmt.Print(txt, "m")
}

func (s Style) RenderAt(in string, x, y int) {
	fmt.Printf("\x1b[%d;%dH", x, y)
	s.Render(in)
}
