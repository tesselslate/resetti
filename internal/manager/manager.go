// Package manager implements various reset "managers" which handle Minecraft
// instances and their changing states.
package manager

import (
	"github.com/woofdoggo/resetti/internal/mc"
)

type Manager interface {
	Wait()
	Start([]mc.Instance, chan error) error
	Stop()
}
