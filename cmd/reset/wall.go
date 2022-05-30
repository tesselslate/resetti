package reset

import (
	"github.com/woofdoggo/resetti/manager"
	"os"
)

func CmdWall() {
	mgr := &manager.WallManager{}
	os.Exit(run("wall", mgr))
}
