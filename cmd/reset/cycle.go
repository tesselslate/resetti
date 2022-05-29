package reset

import (
	"resetti/manager"
)

func CmdCycle() {
	mgr := &manager.StandardManager{}
	run("standard", mgr)
}
