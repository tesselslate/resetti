// Package cfg allows for reading the user's configuration.
package cfg

import "github.com/woofdoggo/resetti/internal/x11"

// Hooks contains various commands to run whenever the user performs certain
// actions.
type Hooks struct {
	Reset      string `toml:"reset"`       // Command to run on ingame reset
	WallLock   string `toml:"wall_lock"`   // Command to run on wall reset
	WallUnlock string `toml:"wall_unlock"` // Command to run on wall unlock
	WallPlay   string `toml:"wall_play"`   // Command to run on wall play
	WallReset  string `toml:"wall_reset"`  // Command to run on wall reset
}

// Keybinds contains the user's keybindings.
type Keybinds struct {
	Focus           x11.Key    `toml:"focus"`             // Focus instance/projector
	Reset           x11.Key    `toml:"reset"`             // Reset all / reset current
	WallLock        x11.Keymod `toml:"wall_lock"`         // (Un)lock instance
	WallPlay        x11.Keymod `toml:"wall_play"`         // Play insatnce
	WallReset       x11.Keymod `toml:"wall_reset"`        // Reset from wall
	WallResetOthers x11.Keymod `toml:"wall_reset_others"` // Focus reset from wall
}

// Obs contains the user's OBS websocket connection information.
type Obs struct {
	Enabled  bool   `toml:"enabled"`  // Mandatory for wall
	Port     uint16 `toml:"port"`     // Connection port
	Password string `toml:"password"` // Password, can be left blank if unused
}

// Wall contains the user's wall settings.
type Wall struct {
	Enabled     bool `toml:"enabled"`      // Whether to use multi or wall
	GotoLocked  bool `toml:"goto_locked"`  // Also known as wall bypass
	GracePeriod int  `toml:"grace_period"` // Milliseconds to wait after preview before a reset can occur

	StretchRes   *Rectangle `toml:"stretch_res"`   // Inactive resolution
	UnstretchRes *Rectangle `toml:"unstretch_res"` // Active resolution
	UseF1        bool       `toml:"use_f1"`

	// Instance hiding (dirt cover) settings.
	Hiding struct {
		// What criteria to use when determining when to show the instance.
		// Only valid options are "percentage" and "delay".
		ShowMethod string `toml:"show_method"`

		// When to show the instances (either milliseconds for delay or
		// generation percentage for percentage.)
		ShowAt int `toml:"show_at"`
	} `toml:"hiding"`

	// Performance settings.
	Performance struct {
		// Optional. Overrides the default sleepbg.lock path ($HOME)
		SleepbgPath string `toml:"sleepbg_path"`

		// Whether or not to use affinity.
		Affinity bool `toml:"affinity"`

		// If enabled, halves the amount of CPU cores available to affinity
		// groups and instead creates double the amount of groups (half for
		// each CCX.)
		CcxSplit bool `toml:"ccx_split"`

		CpusIdle   uint `toml:"affinity_idle"`   // CPUs for idle group
		CpusLow    uint `toml:"affinity_low"`    // CPUs for low group
		CpusMid    uint `toml:"affinity_mid"`    // CPUs for mid group
		CpusHigh   uint `toml:"affinity_high"`   // CPUs for high group
		CpusActive uint `toml:"affinity_active"` // CPUs for active group

		// The number of milliseconds to wait after an instance finishes
		// generating to move it from the mid group to the idle group.
		// A value of 0 disables this functionality.
		BurstLength uint `toml:"burst_length"`

		// The world generation percentage at which instances are moved from
		// the high group to the low group.
		LowThreshold uint `toml:"low_threshold"`
	} `toml:"performance"`
}

// Profile contains an entire configuration profile.
type Profile struct {
	Delay        int    `toml:"delay"`       // Delay between certain actions
	ResetCount   string `toml:"reset_count"` // Reset counter path
	UnpauseFocus bool   `toml:"unpause_focus"`

	Hooks    Hooks    `toml:"hooks"`
	Keybinds Keybinds `toml:"keybinds"`
	Obs      Obs      `toml:"obs"`
	Wall     Wall     `toml:"wall"`
}

// Rectangle is a rectangle. That's it.
type Rectangle struct {
	X, Y, W, H uint32
}
