package reset

import (
	"os"
	"resetti/manager"
)

func CmdWall() {
	mgr := &manager.WallManager{}
	os.Exit(run("wall", mgr))
}
