package mc

import (
	"github.com/fsnotify/fsnotify"
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Instance state types
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

// State contains information about the current state of an instance.
type State struct {
	// Current main state (e.g. dirt, preview)
	Type int

	// World generation progress (0 to 100)
	Progress int

	// Whether or not the instance is in a menu (e.g. pause, inventory).
	// Requires WorldPreview state reader to detect.
	Menu bool
}

// The stateReader interface provides a method for obtaining the state of an
// instance (e.g. generating, previewing, ingame.)
//
// There are currently two implementations: the traditional log reader, and the
// newer wpstateout.txt reader. The wpstateout.txt reader is preferred and
// should be used whenever possible, as it is simpler, faster, and more
// featureful.
type stateReader interface {
	// Path returns the path of the file being read.
	Path() string

	// Process reads any changes to the file and returns any state updates.
	Process() (state State, updated bool, err error)

	// ProcessEvent handles a non-modification change to the file, such as it
	// being deleted or moved. A non-nil error return signals an irrecoverable
	// failure.
	ProcessEvent(fsnotify.Op) error
}

// Update contains a change to the state of a specific instance.
type Update struct {
	State State
	Id    int
}
