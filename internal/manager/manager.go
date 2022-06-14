// Package manager implements various reset "managers" which handle Minecraft
// instances and their changing states.
package manager

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
)

type Manager interface {
	SetConfig(cfg.Config)
	Wait()

	Restart([]mc.Instance) error
	Start([]mc.Instance, chan error) error
	Stop()
}
