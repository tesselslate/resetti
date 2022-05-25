// Package manager provides a "reset manager" which handles incoming events
// from various sources, manages and resets instances, and updates OBS as
// needed.
package manager

import (
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"

	obs "github.com/woofdoggo/go-obs"
)

// Manager is responsible for managing multiple Workers.
type Manager interface {
	Setup(*x11.Client, *obs.Client, cfg.Config)
	Start([]mc.Instance, chan mc.Instance) error
	Stop() error

	GetConfig() cfg.ResetSettings
	GetX() *x11.Client
}

type managerState struct {
	conf cfg.Config
	o    *obs.Client
	x    *x11.Client

	wCmdCh []chan WorkerCommand
	wErrCh chan WorkerError

	mStateCh chan mc.Instance
	mErrCh   chan error

	workers []*Worker
}
