package main

import (
	"fmt"
	"os"
	"resetti/cfg"
	"resetti/manager"
	"resetti/mc"
	"resetti/ui"
	"resetti/x11"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	obs "github.com/woofdoggo/go-obs"
	"gopkg.in/yaml.v2"
)

func main() {
	// Read arguments.
	args := os.Args
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}
	switch args[1] {
	case "standard", "wall":
		run()
	case "key":
		setupKey()
	case "obs":
		setupObs()
	default:
		printHelp()
		os.Exit(1)
	}
}

func run() {
	// Read configuration.
	conf, err := cfg.GetConfig()
	if err != nil {
		if os.IsNotExist(err) {
			writeDefaultConfig()
			os.Exit(1)
		}
		fmt.Printf("Failed to get configuration: %s\n", err)
		os.Exit(1)
	}

	// Start OBS.
	var o *obs.Client
	var obsCh chan error
	if conf.OBS.Enabled {
		o = &obs.Client{}
		url := fmt.Sprintf("localhost:%d", conf.OBS.Port)
		authRequired, errch, err := o.Connect(url)
		if err != nil {
			fmt.Printf("Failed to connect to OBS: %s\n", err)
			os.Exit(1)
		}
		obsCh = errch
		if authRequired {
			err = o.Authenticate(conf.OBS.Password)
			if err != nil {
				fmt.Printf("Failed to authenticate with OBS: %s\n", err)
				os.Exit(1)
			}
		}
	}

	// Connect to the X server.
	x, err := x11.NewClient()
	if err != nil {
		fmt.Printf("Failed to connect to X server: %s\n", err)
		os.Exit(1)
	}

	// Scan for instances.
	instances, err := mc.GetInstances(x)
	if err != nil {
		fmt.Printf("Failed to get instances: %s\n", err)
		os.Exit(1)
	}

	// Start main loop.
	notify := make(chan bool)
	uiCh := make(chan ui.Command, 16)
	prog := tea.NewProgram(ui.NewModel(uiCh, notify))
	go func() {
		<-notify

		// Startup manager.
		var mgr manager.Manager
		stateCh := make(chan mc.Instance, 64)
		switch os.Args[1] {
		case "standard":
			mgr = &manager.StandardManager{}
		default:
			panic("not yet implemented")
		}
		mgr.Setup(x, o, *conf)
		err := mgr.Start(instances, stateCh)
		if err != nil {
			prog.Send(ui.MsgStatus{
				Status: ui.StatusFail,
				Text:   err.Error(),
			})
		}

		prog.Send(ui.MsgStatus{
			Status: ui.StatusOk,
			Text:   "ready",
		})
		prog.Send(instances)
		for {
			select {
			case uiCmd := <-uiCh:
				switch uiCmd {
				case ui.CmdQuit:
					mgr.Stop()
					return
				case ui.CmdRefresh:
					instances, err = mc.GetInstances(x)
					if err != nil {
						prog.Send(ui.MsgStatus{
							Status: ui.StatusFail,
							Text:   err.Error(),
						})
					} else {
						mgr.Stop()
						prog.Send(ui.MsgStatus{
							Status: ui.StatusBusy,
							Text:   "restarting manager...",
						})
						time.Sleep(1 * time.Second)
						err = mgr.Start(instances, stateCh)
						if err != nil {
							prog.Send(ui.MsgStatus{
								Status: ui.StatusFail,
								Text:   err.Error(),
							})
							prog.Send([]mc.Instance{})
						} else {
							prog.Send(instances)
							prog.Send(ui.MsgStatus{
								Status: ui.StatusOk,
								Text:   "ready",
							})
						}
					}
				}
			case obsErr := <-obsCh:
				// Receiving an error from the OBS client means that a
				// fatal error occurred and it has stopped.
				prog.Send(ui.MsgStatus{
					Status: ui.StatusFail,
					Text:   fmt.Sprintf("OBS: %s", obsErr.Error()),
				})
			case state := <-stateCh:
				prog.Send(state)
			}
		}
	}()

	// Start UI.
	if err := prog.Start(); err != nil {
		fmt.Printf("Tea error: %s\n", err)
		os.Exit(1)
	}
}

func setupKey() {
	if len(os.Args) < 3 {
		printHelp()
		os.Exit(1)
	}
	if os.Args[2] != "reset" && os.Args[2] != "focus" {
		fmt.Println("Unrecognized key.")
		fmt.Println("Please use 'reset' or 'focus'.")
		os.Exit(1)
	}

	conf, err := cfg.GetConfig()
	if err != nil {
		if os.IsNotExist(err) {
			conf = &cfg.DefaultConfig
		} else {
			fmt.Printf("Failed to get configuration: %s\n", err)
			os.Exit(1)
		}
	}

	x, err := x11.NewClient()
	if err != nil {
		fmt.Printf("Failed to connect to X server: %s\n", err)
		os.Exit(1)
	}
	err = x.GrabKeyboard()
	if err != nil {
		fmt.Printf("Failed to grab keyboard: %s\n", err)
		os.Exit(1)
	}

	_, keych := x.Loop()
	stopch := make(chan bool, 1)
	var key *x11.Key
	mx := sync.Mutex{}
	go func() {
		mx.Lock()
		defer mx.Unlock()
		for {
			select {
			case evt := <-keych:
				if evt.State == x11.KeyDown {
					k := evt.Key
					key = &k
				}
			case <-stopch:
				return
			}
		}
	}()

	fmt.Println("Hold down your desired keybinding.")
	fmt.Println("Please wait 3 seconds.")
	time.Sleep(3 * time.Second)
	x.UngrabKeyboard()

	// Wait for the key listener goroutine to release the lock.
	stopch <- true
	mx.Lock()

	if key == nil {
		fmt.Println("No keypress detected!")
		os.Exit(1)
	}
	switch os.Args[2] {
	case "reset":
		conf.Keys.Reset = *key
	case "focus":
		conf.Keys.Focus = *key
	}
	_, err = writeConfig(conf)
	if err != nil {
		fmt.Printf("Failed to write config:\n%s\n", err)
		os.Exit(1)
	}
	fmt.Println("Done!")
	os.Exit(0)
}

func setupObs() {
	// TODO
}

func printHelp() {
	fmt.Println(`
    resetti - Minecraft resetting macro

    USAGE:
        resetti standard            Run the "standard" resetter. Cycles
                                    between instances sequentially.
                                    Supports both single- and multi-instance.

        resetti wall                Run the "wall" style resetter.
                                    Requires OBS.

    CONFIGURATION:
        resetti key [reset|focus]   Set your keybinds for using resetti.
        resetti obs                 Setup OBS scenes for resetti.
    `)
}

func writeConfig(c *cfg.Config) (string, error) {
	confPath, err := cfg.GetPath()
	if err != nil {
		return "", fmt.Errorf("could not locate config dir: %s", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("failed to serialize config: %s", err)
	}
	err = os.WriteFile(confPath, data, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write default config: %s", err)
	}
	return confPath, nil
}

func writeDefaultConfig() {
	confPath, err := writeConfig(&cfg.DefaultConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("No configuration file found")
	fmt.Printf("Wrote default to:\n  %s\n", confPath)
	fmt.Println("Modify as needed, then launch resetti again.")
}
