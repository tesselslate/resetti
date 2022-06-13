package reset

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/manager"
	"github.com/woofdoggo/resetti/mc"
	"github.com/woofdoggo/resetti/obs"
	"github.com/woofdoggo/resetti/ui"
	"github.com/woofdoggo/resetti/x11"
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
	var resetHandle *os.File
	if conf.CountPath != "" {
		resetHandle, err = os.OpenFile(conf.CountPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			fmt.Println("Failed to open reset count:", err)
			os.Exit(1)
		}
		defer resetHandle.Close()
	}
	if mode == "wall" && !conf.OBS.Enabled {
		fmt.Println("OBS integration must be enabled for wall.")
		fmt.Println("Please update your configuration.")
		os.Exit(1)
	}
	var obserr <-chan error
	if conf.OBS.Enabled {
		errch, err := obs.Initialize(conf.OBS.Port, conf.OBS.Password)
		obserr = errch
		if err != nil {
			fmt.Println("Failed to connect to OBS:", err)
			os.Exit(1)
		}
	}
	xerr := make(chan error, 32)
	x11.Subscribe(xerr, nil)
	err = x11.Initialize()
	if err != nil {
		fmt.Println("Failed to connect to X server:", err)
		os.Exit(1)
	}
	instances, err := mc.GetInstances()
	if err != nil {
		fmt.Println("Failed to get Minecraft instances:", err)
		os.Exit(1)
	}
	u := ui.Ui{}
	u.Start(instances, resetHandle)
	mgr.SetConfig(*conf)
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
			x11.Close()
			return 0
		case err := <-mgrErrors:
			ui.LogError("Fatal manager error: %s", err)
			mgr.Wait()
			ui.Log("Attempting to reboot manager...")
			time.Sleep(1 * time.Second)
			instances, err := mc.GetInstances()
			if err != nil {
				ui.LogError("Failed to get Minecraft instances: %s", err)
				u.Stop()
				x11.Close()
				return 1
			}
			err = mgr.Start(instances, mgrErrors)
			if err != nil {
				ui.LogError("Failed to restart manager: %s", err)
				u.Stop()
				x11.Close()
				return 1
			}
		case err := <-obserr:
			ui.LogError("OBS websocket error: %s", err)
		case err := <-xerr:
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
			x11.Close()
			return 1
		}
	}
}
