package wall

import (
	"fmt"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type StandardController struct {
	conf       *cfg.Profile
	obs        *obs.Client
	wallWidth  int
	wallHeight int
	instWidth  int
	instHeight int

	states []mc.InstanceState
}

func (c *StandardController) GetInstanceId(x, y int) int {
	return ((y / c.instHeight) * c.wallWidth) + x/c.instWidth
}

func (c *StandardController) GetResetAllInstances() []int {
	list := make([]int, len(c.states))
	for i := range list {
		list[i] = i
	}
	return list
}

func (c *StandardController) Lock(id int) error {
	c.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Lock %d", id+1), true)
	return nil
}

func (c *StandardController) Setup(obs *obs.Client, conf *cfg.Profile, states []mc.InstanceState) error {
	appendUnique := func(slice []float64, item float64) []float64 {
		for _, v := range slice {
			if item == v {
				return slice
			}
		}
		return append(slice, item)
	}
	xs, ys := make([]float64, 0), make([]float64, 0)
	for i := 0; i < len(states); i++ {
		x, y, _, _, err := obs.GetSceneItemTransform(
			"Wall",
			fmt.Sprintf("Wall MC %d", i+1),
		)
		if err != nil {
			return err
		}
		xs = appendUnique(xs, x)
		ys = appendUnique(ys, y)
	}
	c.obs = obs
	c.conf = conf
	c.wallWidth = len(xs)
	c.wallHeight = len(ys)
	c.states = make([]mc.InstanceState, len(states))
	copy(c.states, states)
	return nil
}

func (c *StandardController) Unlock(id int) error {
	c.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Lock %d", id+1), false)
	return nil
}

func (c *StandardController) Update(update mc.Update) error {
	prev := c.states[update.Id]
	next := update.State
	nowDirt := next.State == mc.StDirt && prev.State != mc.StDirt
	if c.conf.AdvancedWall.PreviewFreezing {
		if next.Progress >= c.conf.AdvancedWall.FreezeThreshold {
			c.obs.SetSourceFilterEnabledAsync(
				fmt.Sprintf("Wall MC %d", update.Id+1),
				fmt.Sprintf("Freeze %d", update.Id+1),
				true,
			)
		}
		if nowDirt {
			c.obs.SetSourceFilterEnabledAsync(
				fmt.Sprintf("Wall MC %d", update.Id+1),
				fmt.Sprintf("Freeze %d", update.Id+1),
				false,
			)
		}
	}
	if c.conf.AdvancedWall.InstanceHiding {
		if nowDirt {
			c.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), false)
		}
		if next.Progress > 0 {
			c.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), true)
		}
	}
	c.states[update.Id] = update.State
	return nil
}

func (c *StandardController) UpdateProjector(width, height int) {
	c.instWidth = width / c.wallWidth
	c.instHeight = height / c.wallHeight
}
