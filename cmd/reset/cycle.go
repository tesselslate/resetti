package reset

import (
	"os"
	"resetti/manager"
)

func CmdCycle() {
	mgr := &manager.StandardManager{}
	os.Exit(run("standard", mgr))
}
