package reset

import (
	"github.com/woofdoggo/resetti/manager"
	"os"
)

func CmdCycle() {
	mgr := &manager.StandardManager{}
	os.Exit(run("standard", mgr))
}
