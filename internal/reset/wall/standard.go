package wall

import (
	"fmt"

	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type StandardController struct {
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
	return c.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", id+1), true)
}

func (c *StandardController) Setup(obs *obs.Client, states []mc.InstanceState) error {
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
			fmt.Sprintf("MC %d", i+1),
		)
		if err != nil {
			return err
		}
		xs = appendUnique(xs, x)
		ys = appendUnique(ys, y)
	}
	c.obs = obs
	c.wallWidth = len(xs)
	c.wallHeight = len(ys)
	c.states = make([]mc.InstanceState, len(states))
	copy(c.states, states)
	return nil
}

func (c *StandardController) Unlock(id int) error {
	return c.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", id+1), false)
}

func (c *StandardController) Update(update mc.Update) error {
	// TODO: Instance hiding, preview freezing
	c.states[update.Id] = update.State
	return nil
}

func (c *StandardController) UpdateProjector(width, height int) {
	c.instWidth = width / c.wallWidth
	c.instHeight = height / c.wallHeight
}
