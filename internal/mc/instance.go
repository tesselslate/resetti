package mc

import (
	"log"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
)

// The set of possible states an instance can be in.
const (
	StMenu    int = iota // Main menu
	StDirt               // Dirt world generation screen
	StPreview            // World preview
	StIdle               // World generation finished
	StIngame             // Currently being played
)

// InstanceInfo contains information about how to interact with a Minecraft
// instance, such as its game directory and window ID.
type InstanceInfo struct {
	Id         int           // Instance number
	Pid        uint32        // Process ID
	Wid        xproto.Window // Window ID
	Dir        string        // .minecraft directory
	Version    int           // Minecraft version
	ResetKey   x11.Key       // Atum reset key
	PreviewKey x11.Key       // Leave preview key
}

// InstanceState contains information about the current state of the instance.
type InstanceState struct {
	State    int // Current state of the instance
	Progress int // World generation progress (0 to 100)
}

// Instance represents a single Minecraft instance and takes care of reading its
// log file and performing actions on it.
type Instance struct {
	InstanceInfo
	conf *cfg.Profile
	time xproto.Timestamp // the last time a key event was sent
	x    *x11.Client
}

// NewInstance creates a new Instance from the given instance information.
func NewInstance(info InstanceInfo, conf *cfg.Profile, x *x11.Client) Instance {
	return Instance{
		InstanceInfo: info,
		conf:         conf,
		x:            x,
	}
}

// Click clicks on the instance's window.
func (i *Instance) Click() error {
	return i.x.Click(i.Wid)
}

// Focus focuses the instance's window. If an error occurs, it will be logged.
func (i *Instance) Focus() {
	if err := i.x.FocusWindow(i.Wid); err != nil {
		log.Printf("Instance %d failed to focus: %s\n", i.Id, err)
	}
}

// FocusAndUnpause focuses the instance's window and then unpauses if the user
// has set `unpause_on_focus` in their config. If an error occurs, it will be
// logged.
func (i *Instance) FocusAndUnpause(timestamp xproto.Timestamp, idle bool) {
	i.Focus()

	time.Sleep(time.Millisecond * time.Duration(i.conf.Reset.Delay))
	if i.conf.Reset.UnpauseFocus && idle {
		i.x.SendKeyPress(
			x11.KeyEscape,
			i.Wid,
			i.lastTime(timestamp),
		)
	}
	if i.conf.Wall.HideGui && i.conf.General.ResetType == "wall" {
		i.x.SendKeyPress(
			x11.KeyF1,
			i.Wid,
			i.lastTime(timestamp),
		)
	}
}

// PressEsc presses escape to [un]pause the instance. If an error occurs, it will
// be logged.
func (i *Instance) PressEsc(timestamp xproto.Timestamp) {
	i.x.SendKeyPress(x11.KeyEscape, i.Wid, i.lastTime(timestamp))
}

// PressF3Esc presses F3+Escape to pause the instance without the pause menu.
// If an error occurs, it will be logged.
func (i *Instance) PressF3Esc(timestamp xproto.Timestamp) {
	i.x.SendKeyDown(x11.KeyF3, i.Wid, i.lastTime(timestamp))
	i.x.SendKeyPress(x11.KeyEscape, i.Wid, i.lastTime(timestamp))
	i.x.SendKeyUp(x11.KeyF3, i.Wid, i.lastTime(timestamp))
}

// PressF3 presses F3 to hide the pie chart.
func (i *Instance) PressF3(timestamp xproto.Timestamp) {
	i.x.SendKeyPress(x11.KeyF3, i.Wid, i.lastTime(timestamp))
}

// Reset presses the instance's reset key. If an error occurs, it will be
// logged.
func (i *Instance) Reset(timestamp xproto.Timestamp) {
	i.x.SendKeyPress(i.ResetKey.Code, i.Wid, i.lastTime(timestamp))
}

// Stretch stretches the instance's window.
func (i *Instance) Stretch(conf cfg.Profile) error {
	if !conf.Wall.StretchWindows {
		return nil
	}
	return i.x.MoveWindow(
		i.Wid,
		conf.Wall.ResizeX,
		conf.Wall.ResizeY,
		conf.Wall.StretchWidth,
		conf.Wall.StretchHeight,
	)
}

// Unstretch resizes the window back to its normal dimensions.
func (i *Instance) Unstretch(conf cfg.Profile) error {
	if !conf.Wall.StretchWindows {
		return nil
	}
	return i.x.MoveWindow(
		i.Wid,
		conf.Wall.ResizeX,
		conf.Wall.ResizeY,
		conf.Wall.UnstretchWidth,
		conf.Wall.UnstretchHeight,
	)
}

// lastTime returns the maximum of the given timestamp and the last stored
// timestamp. If the given timestamp is later, then the last stored timestamp
// is set to that value.
func (i *Instance) lastTime(timestamp xproto.Timestamp) *xproto.Timestamp {
	if timestamp > i.time {
		i.time = timestamp
	}
	return &i.time
}
