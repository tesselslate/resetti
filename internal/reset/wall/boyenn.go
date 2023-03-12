package wall

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type BoyennController struct {
}

func (*BoyennController) GetInstanceId(x int, y int) int {
	panic("unimplemented")
}

func (*BoyennController) GetResetAllInstances() []int {
	panic("unimplemented")
}

func (*BoyennController) Lock(int) error {
	panic("unimplemented")
}

func (*BoyennController) Setup(*obs.Client, *cfg.Profile, []mc.InstanceState) error {
	panic("unimplemented")
}

func (*BoyennController) Unlock(int) error {
	panic("unimplemented")
}

func (*BoyennController) Update(mc.Update) error {
	panic("unimplemented")
}

func (*BoyennController) UpdateProjector(width int, height int) {
	panic("unimplemented")
}
