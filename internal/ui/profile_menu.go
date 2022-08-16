package ui

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/woofdoggo/resetti/internal/cfg"
)

// ShowProfileMenu displays the profile selection menu to the user and returns
// their choice. If no choice was picked, then the returned string is empty.
func ShowProfileMenu() (string, error) {
	choices := make([]string, 0)
	current := 0
	cfgDir, err := cfg.GetFolder()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(cfgDir)
	if err != nil {
		return "", err
	}
	for _, v := range entries {
		if v.IsDir() {
			continue
		}
		name := v.Name()
		if name[0] == '.' {
			continue
		}
		if strings.HasSuffix(name, ".toml") {
			choices = append(choices, strings.TrimSuffix(name, ".toml"))
		}
	}
	if len(choices) == 0 {
		return "", errors.New("no configuration profiles found - make one")
	}
	keys := listen(context.Background())
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGWINCH)
	width, height, err := getSize()
	if err != nil {
		return "", err
	}
	err = initTerminal()
	if err != nil {
		return "", err
	}
	defer finiTerminal()
	draw := func() {
		clearTerminal()
		const HELP_STR = "enter: select | ctrl-c: quit"

		// Calculate UI size.
		cols := 0
		for _, v := range choices {
			if len(v) > cols {
				cols = len(v)
			}
		}
		if len(HELP_STR) > cols {
			cols = len(HELP_STR)
		}
		cols += 4
		if cols > 40 {
			cols = 40
		}
		rows := len(choices) + 2
		sx, sy := width/2-cols/2, height/2-rows/2
		if width < cols || height < rows || sx < 3 || sy < 1 {
			newStyle().RenderAt("Terminal too small", 0, 0)
			return
		}
		newStyle().Foreground(Cyan).RenderAt(HELP_STR, sx, sy+rows)
		newStyle().Foreground(BrightYellow).Bold().RenderAt("Profiles", sx, sy)
		for i, choice := range choices {
			style := newStyle()
			if i == current {
				style = style.Foreground(BrightMagenta).Bold()
				style.RenderAt(">", sx-2, sy+i+1)
			} else {
				style = style.Foreground(White)
			}
			str := choice
			if len(str) > cols-3 {
				str = str[:cols-6] + "..."
			}
			style.RenderAt(str, sx, sy+i+1)
		}
	}
	draw()
	for {
		select {
		case k := <-keys:
			switch k {
			case KeyUp:
				if current == 0 {
					continue
				}
				current -= 1
				draw()
			case KeyDown:
				if current == len(choices)-1 {
					continue
				}
				current += 1
				draw()
			case KeyEnter:
				return choices[current], nil
			case KeyCtrlC:
				return "", nil
			}
		case <-sigs:
			width, height, err = getSize()
			if err == nil {
				draw()
			}
		}
	}
}
