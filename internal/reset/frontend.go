package reset

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Frontend handles user input and switches instances, manages OBS output,
// and so on.
type Frontend interface {
	// Handle a single user input.
	HandleInput(x11.Event) error

	// Handle a single state update.
	HandleUpdate(mc.Update) error

	// Setup the Frontend to receive requests.
	Setup(FrontendOptions) error

	// Whether or not the instance with the given ID should be paused.
	ShouldPause(int) bool
}

// FrontendOptions contains dependencies for setting up a Frontend.
type FrontendOptions struct {
	Conf       *cfg.Profile
	Controller *Controller
	Obs        *obs.Client
	X          *x11.Client
	States     []mc.InstanceState
	Instances  []mc.Instance
}
