// Package manager implements various reset "managers" which handle Minecraft
// instances and their changing states.
package manager

import (
	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/mc"
)

type Manager interface {
	SetConfig(cfg.Config)
	Wait()

	Restart([]mc.Instance) error
	Start([]mc.Instance, chan error) error
	Stop()
}
