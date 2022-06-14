package ui

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/internal/terminal"
)

type profileMenu struct {
	choices []string
	current int
	width   int
	height  int
}

// ShowProfileMenu displays the profile selection menu to the user and
// returns their choice.
func ShowProfileMenu() (string, error) {
	p := profileMenu{}
	p.choices = make([]string, 0)
	cfgDir, err := cfg.GetPath()
	if err != nil {
		return "", err
	}
	keys := make(chan terminal.Key, 32)
	dirList, err := os.ReadDir(cfgDir)
	if err != nil {
		return "", err
	}
	for _, v := range dirList {
		if v.IsDir() {
			continue
		}
		name := v.Name()
		if name[0] == '.' {
			continue
		}
		if strings.HasSuffix(name, ".toml") {
			p.choices = append(p.choices, strings.TrimSuffix(name, ".toml"))
		}
	}
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGWINCH)
	w, h, err := terminal.GetSize()
	if err != nil {
		return "", err
	}
	p.width = w
	p.height = h
	err = terminal.Init(keys)
	if err != nil {
		return "", err
	}
	defer terminal.Fini()
	p.Draw()
	for {
		select {
		case k := <-keys:
			switch k {
			case terminal.KeyUp:
				if p.current == 0 {
					continue
				}
				p.current -= 1
				p.Draw()
			case terminal.KeyDown:
				if p.current == len(p.choices)-1 {
					continue
				}
				p.current += 1
				p.Draw()
			case terminal.KeyEnter:
				return p.choices[p.current], nil
			case terminal.KeyCtrlC:
				return "", nil
			}
		case <-sigs:
			p.width = w
			p.height = h
			p.Draw()
		}
	}
}

func (p *profileMenu) Draw() {
	terminal.Clear()
	rows := len(p.choices) + 3
	if p.width < 30 || p.height < rows {
		terminal.NewStyle().RenderAt("Terminal too small", 0, 0)
		return
	}
	sx, sy := p.width/2-15, p.height/2-rows/2
	terminal.NewStyle().Foreground(terminal.Cyan).RenderAt("enter: select | ctrl-c: quit", sx, sy+rows)
	terminal.NewStyle().Foreground(terminal.BrightYellow).Bold().RenderAt("Profiles", sx, sy)
	for i, v := range p.choices {
		style := terminal.NewStyle()
		if i == p.current {
			style = style.Foreground(terminal.BrightMagenta).Bold()
			style.RenderAt(">", sx-2, sy+i+1)
		} else {
			style = style.Foreground(terminal.White)
		}
		str := v
		if len(str) > 30 && p.width == 30 {
			str = str[:27] + "..."
		}
		style.RenderAt(str, sx, sy+i+1)
	}
}
