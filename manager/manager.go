// Package manager implements various reset "managers" which handle Minecraft
// instances and their changing states.
package manager

import (
	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/mc"

	obs "github.com/woofdoggo/go-obs"
)

type Manager interface {
	SetConfig(cfg.Config)
	SetDeps(*obs.Client)
	Wait()

	Restart([]mc.Instance) error
	Start([]mc.Instance, chan error) error
	Stop()
}
