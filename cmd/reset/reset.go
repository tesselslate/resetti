package reset

import (
	"fmt"
	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/manager"
	"github.com/woofdoggo/resetti/mc"
	"github.com/woofdoggo/resetti/ui"
	"github.com/woofdoggo/resetti/x11"
	"os"
	"os/signal"
	"syscall"
	"time"

	obs "github.com/woofdoggo/go-obs"
)

func run(mode string, mgr manager.Manager) int {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fmt.Println("Failed to get log path:", err)
		os.Exit(1)
	}
	logHandle, err := os.OpenFile(cacheDir+"/resetti.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		os.Exit(1)
	}
	ui.SetLogWriter(logHandle)
	defer logHandle.Close()
	conf, err := cfg.GetConfig()
	if err != nil {
		fmt.Println("Failed to read config:", err)
		os.Exit(1)
	}
	resetHandle, err := os.OpenFile(conf.CountPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("Failed to open reset count:", err)
		os.Exit(1)
	}
	defer resetHandle.Close()
	if mode == "wall" && !conf.OBS.Enabled {
		fmt.Println("OBS integration must be enabled for wall.")
		fmt.Println("Please update your configuration.")
		os.Exit(1)
	}
	var o *obs.Client
	var obsErrors chan error
	if conf.OBS.Enabled {
		o = &obs.Client{}
		authRequired, errch, err := o.Connect(fmt.Sprintf("localhost:%d", conf.OBS.Port))
		if err != nil {
			fmt.Println("Failed to connect to OBS:", err)
			os.Exit(1)
		}
		obsErrors = errch
		if authRequired {
			err = o.Login(conf.OBS.Password)
			if err != nil {
				fmt.Println("Failed to authenticate with OBS:", err)
				os.Exit(1)
			}
		}
	}
	x, err := x11.NewClient()
	if err != nil {
		fmt.Println("Failed to connect to X server:", err)
		os.Exit(1)
	}
	x.Loop()
	instances, err := mc.GetInstances(x)
	if err != nil {
		fmt.Println("Failed to get Minecraft instances:", err)
		os.Exit(1)
	}
	u := ui.Ui{}
	u.Start(instances, resetHandle)
	mgr.SetConfig(*conf)
	mgr.SetDeps(x, o)
	mgrErrors := make(chan error)
	err = mgr.Start(instances, mgrErrors)
	if err != nil {
		fmt.Println("Failed to start manager:", err)
		u.Stop()
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ui.Log("Started up!")
	ui.Log("Session type: %s", mode)
	for {
		select {
		case <-signals:
			ui.Log("Shutting down.")
			u.Stop()
			mgr.Stop()
			x.LoopStop()
			return 0
		case err := <-mgrErrors:
			ui.LogError("Fatal manger error: %s", err)
			mgr.Wait()
			ui.Log("Attempting to reboot manager...")
			time.Sleep(1 * time.Second)
			instances, err := mc.GetInstances(x)
			if err != nil {
				ui.LogError("Failed to get Minecraft instances: %s", err)
				u.Stop()
				x.LoopStop()
				return 1
			}
			err = mgr.Start(instances, mgrErrors)
			if err != nil {
				ui.LogError("Failed to restart manager: %s", err)
				u.Stop()
				x.LoopStop()
				return 1
			}
		case err := <-obsErrors:
			ui.LogError("OBS websocket error: %s", err)
		case err := <-x.Errors:
			if err == x11.ErrConnectionDied {
				ui.LogError("X connection died.")
				ui.Log("Shutting down.")
				u.Stop()
				mgr.Stop()
				return 1
			} else {
				ui.LogError("X error: %s", err)
			}
		case err := <-u.Errors:
			ui.LogError("UI error: %s", err)
			ui.Log("Shutting down.")
			fmt.Printf("UI error: %s\n", err)
			mgr.Stop()
			x.LoopStop()
			return 1
		}
	}
}
