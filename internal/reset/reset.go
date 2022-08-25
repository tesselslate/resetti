package reset

import (
	"context"
	"fmt"

	go_obs "github.com/woofdoggo/go-obs"
	"github.com/woofdoggo/resetti/internal/cfg"
)

// connectObs attempts to connect to OBS.
func connectObs(conf cfg.Profile, instanceCount int) (*go_obs.Client, chan error, error) {
	obs := &go_obs.Client{}
	needsAuth, obsErr, err := obs.Connect(fmt.Sprintf("localhost:%d", conf.Obs.Port))
	if err != nil {
		return nil, nil, err
	}
	if needsAuth {
		err := obs.Login(conf.Obs.Password)
		if err != nil {
			return nil, nil, err
		}
	}
	err = setSceneCollection(obs, fmt.Sprintf("resetti - %d multi", instanceCount))
	if err != nil {
		return nil, nil, err
	}
	return obs, obsErr, nil
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
