package reset

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// clickInstances clicks each instance once to avoid the Atum bug.
func clickInstances(x *x11.Client, instances []Instance) error {
	for _, v := range instances {
		if err := x.Click(v.Wid); err != nil {
			return err
		}
	}
	return nil
}

// connectObs attempts to connect to OBS.
func connectObs(ctx context.Context, conf cfg.Profile, instanceCount int) (*obs.Client, <-chan error, error) {
	obs := &obs.Client{}
	errch, err := obs.Connect(ctx, fmt.Sprintf("localhost:%d", conf.Obs.Port), conf.Obs.Password)
	if err != nil {
		return nil, nil, err
	}
	err = obs.SetSceneCollection(fmt.Sprintf("resetti - %d multi", instanceCount))
	if err != nil {
		return nil, nil, err
	}
	return obs, errch, nil
}

// startLogReaders creates log reader goroutines for each instance.
func startLogReaders(instances []Instance) (<-chan LogUpdate, context.CancelFunc, error) {
	updates := make(chan LogUpdate, UPDATE_CHANNEL_SIZE)
	parentCtx, cancelFunc := context.WithCancel(context.Background())
	for i, inst := range instances {
		ctx, instCancel := context.WithCancel(parentCtx)
		ch, err := readLog(inst, ctx)
		if err != nil {
			// Calling cancelFunc is enough to cancel the just-created context,
			// but Go will still warn of a context leak if instCancel is not
			// called.
			instCancel()
			cancelFunc()
			return nil, nil, err
		}
		go func(id int) {
			for {
				update, more := <-ch
				updates <- LogUpdate{
					Id:    id,
					State: update,
					Done:  !more,
				}
				if !more {
					instCancel()
					return
				}
			}
		}(i)
	}
	return updates, cancelFunc, nil
}

// printDebugInfo prints some debug information to the log.
func printDebugInfo(x *x11.Client, conf cfg.Profile, instances []Instance) {
	// Print debug information.
	serializedConf, err := json.Marshal(conf)
	if err != nil {
		log.Println("Failed to print configuration")
	}
	log.Printf("Config:\n%s\n", string(serializedConf))
	log.Printf("Running %d instances\n", len(instances))
	log.Printf("Root: %d\n", x.RootWindow())
	log.Println("WM properties:")
	log.Printf("_NET_WM_NAME: %s", x.GetWmName())
	log.Printf("_NET_SUPPORTED: %s", x.GetWmSupported())
	for id, inst := range instances {
		log.Printf(
			"Instance %d, wid %d, pid %d version %d\n",
			id,
			inst.Wid,
			inst.Pid,
			inst.Version,
		)
		dir, err := os.ReadDir(inst.Dir + "/mods")
		if err != nil {
			log.Printf("Failed to get mods: %s\n", err)
			continue
		}
		for _, entry := range dir {
			name := entry.Name()
			atum := strings.Contains(name, "atum")
			fastreset := strings.Contains(name, "fast-reset")
			worldpreview := strings.Contains(name, "worldpreview")
			if atum || fastreset || worldpreview {
				log.Println(name)
			}
		}
	}
}
