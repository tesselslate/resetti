package setup

import (
	"bufio"
	"fmt"
	"os"
	"resetti/cfg"
	"resetti/x11"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
	"gopkg.in/yaml.v2"
)

func CmdKeys() {
	x, err := x11.NewClient()
	x.Loop()
	if err != nil {
		fmt.Println("Failed to connect to X server:", err)
		return
	}
	conf, err := cfg.GetConfig()
	if err != nil {
		fmt.Println("Failed to read config:", err)
		fmt.Println("Consider preparing the default config with:")
		fmt.Println("resetti --save-default")
		return
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("\n\n")
		fmt.Println("Pick a key to configure.")
		fmt.Println("Once selected, press any key modifiers (e.g. Shift and/or Control)")
		fmt.Println("you would like to use for the given action. Your choice will be")
		fmt.Println("saved after 3 seconds.")
		fmt.Println("1: Play Instance")
		fmt.Println("2: Reset Instance")
		fmt.Println("3: Play Instance, Reset Others")
		fmt.Println("q: Save and Exit Configuration")
		res, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read input.")
			return
		}
		switch res[0] {
		case '1', '2', '3':
			fmt.Println("Waiting...")
			x.GrabKeyboard()
			keys := make(map[xproto.Keycode]bool)
			timeout := time.After(3 * time.Second)
		loop:
			for {
				select {
				case evt := <-x.Keys:
					if evt.State == x11.KeyDown {
						keys[evt.Key.Code] = true
					}
				case <-timeout:
					fmt.Println("Done!")
					break loop
				}
			}
			x.UngrabKeyboard()
			modlist := make([]string, 0)
			mod := x11.Keymod(x11.ModNone)
			for k := range keys {
				switch k {
				case x11.KeyShift:
					modlist = append(modlist, "Shift")
					mod |= x11.ModShift
				case x11.KeyCtrl:
					modlist = append(modlist, "Control")
					mod |= x11.ModCtrl
				}
			}
			if len(modlist) == 0 {
				fmt.Println("Set action to occur on normal click (no key modifiers).")
			} else {
				fmt.Printf("Set action to occur on %s click.\n", strings.Join(modlist, "+"))
			}
			switch res[0] {
			case '1':
				conf.Wall.Play = mod
			case '2':
				conf.Wall.Reset = mod
			case '3':
				conf.Wall.ResetOthers = mod
			}
		case 'q':
			confBytes, err := yaml.Marshal(&conf)
			if err != nil {
				fmt.Println("Failed to serialize config:", err)
				return
			}
			confPath, err := cfg.GetPath()
			if err != nil {
				fmt.Println("Failed to get config path:", err)
				return
			}
			err = os.WriteFile(confPath, confBytes, 0644)
			if err != nil {
				fmt.Println("Failed to write config:", err)
				return
			}
			fmt.Println("Done!")
			return
		default:
			fmt.Println("Please pick a valid option.")
		}
	}
}
