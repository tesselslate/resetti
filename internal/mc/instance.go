package mc

import (
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/x11"
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
