package reset

import "resetti/manager"

func CmdWall() {
	mgr := &manager.WallManager{}
	run("wall", mgr)
}
