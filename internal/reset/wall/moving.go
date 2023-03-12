package wall

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type MovingController struct {
}

func (*MovingController) GetInstanceId(x int, y int) int {
	panic("unimplemented")
}

func (*MovingController) GetResetAllInstances() []int {
	panic("unimplemented")
}

func (*MovingController) Lock(int) error {
	panic("unimplemented")
}

func (*MovingController) Setup(*obs.Client, *cfg.Profile, []mc.InstanceState) error {
	panic("unimplemented")
}

func (*MovingController) Unlock(int) error {
	panic("unimplemented")
}

func (*MovingController) Update(mc.Update) error {
	panic("unimplemented")
}

func (*MovingController) UpdateProjector(width int, height int) {
	panic("unimplemented")
}
