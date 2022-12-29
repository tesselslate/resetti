package mc

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/sys/unix"
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
	Id       int           // Instance number
	Pid      uint32        // Process ID
	Wid      xproto.Window // Window ID
	Dir      string        // .minecraft directory
	Version  int           // Minecraft version
	ResetKey x11.Key       // Atum reset key
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

// Pause presses F3+Escape to pause the instance. If an error occurs, it will
// be logged.
func (i *Instance) Pause(timestamp xproto.Timestamp) {
	i.x.SendKeyDown(x11.KeyF3, i.Wid, i.lastTime(timestamp))
	i.x.SendKeyPress(x11.KeyEscape, i.Wid, i.lastTime(timestamp))
	i.x.SendKeyUp(x11.KeyF3, i.Wid, i.lastTime(timestamp))
}

// Reset presses the instance's reset key. If an error occurs, it will be
// logged.
func (i *Instance) Reset(timestamp xproto.Timestamp) {
	i.x.SendKeyPress(i.ResetKey.Code, i.Wid, i.lastTime(timestamp))
}

// SetAffinity sets the CPU affinity for all of the instance's threads to the
// given CPU mask.
func (i *Instance) SetAffinity(cpus *unix.CPUSet) error {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", i.Pid))
	if err != nil {
		return errors.Wrap(err, "read dir")
	}
	for _, entry := range entries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		// It's possible that a thread was killed since we read the directory.
		// Return the error only if it is not an ERSCH (no such process.)
		if err = unix.SchedSetaffinity(tid, cpus); err != syscall.Errno(3) && err != nil {
			return errors.Wrap(err, "sched_setaffinity")
		}

	}
	return nil
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

// Unpause presses escape to unpause the instance. If an error occurs, it will
// be logged.
func (i *Instance) Unpause(timestamp xproto.Timestamp) {
	i.x.SendKeyPress(x11.KeyEscape, i.Wid, i.lastTime(timestamp))
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
