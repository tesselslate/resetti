package wall

import (
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// ObsController handles the OBS output for wall and tells the wall frontend
// which instances to perform actions on.
type ObsController interface {
	// GetInstanceId returns the ID of the instance at the given coordinates on
	// the wall scene.
	GetInstanceId(x, y int) int

	// GetResetAllInstances returns the list of instance IDs to reset with the
	// reset all keybind.
	GetResetAllInstances() []int

	// Lock locks the given instance.
	Lock(int) error

	// Setup sets up the ObsController.
	Setup(*obs.Client, []mc.InstanceState) error

	// Unlock unlocks the given instance.
	Unlock(int) error

	// Update processes an instance state update.
	Update(mc.Update) error

	// UpdateProjector updates the size of the projector window.
	UpdateProjector(width, height int)
}
