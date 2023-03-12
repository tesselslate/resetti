package reset

import (
	"log"
	"os/exec"
	"strings"

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

// runHook runs the given command.
func runHook(str string) {
	if str == "" {
		return
	}
	splits := strings.Split(str, " ")
	var cmd *exec.Cmd
	if len(splits) == 1 {
		cmd = exec.Command(splits[0])
	} else {
		cmd = exec.Command(splits[0], splits[1:]...)
	}
	err := cmd.Run()
	if err != nil {
		log.Printf("runHook err: %s\n", err)
		return
	}
}
