package wall

import (
	"fmt"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type StandardController struct {
	conf          *cfg.Profile
	obs           *obs.Client
	addons        []DisplayAddon
	wallWidth     int
	wallHeight    int
	instWidth     int
	instHeight    int
	instanceCount int
}

func (c *StandardController) GetInstanceId(x, y int) int {
	return ((y / c.instHeight) * c.wallWidth) + x/c.instWidth
}

func (c *StandardController) GetResetAllInstances() []int {
	list := make([]int, c.instanceCount)
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
	c.wallWidth = len(xs)
	c.wallHeight = len(ys)
	c.obs = obs
	c.conf = conf
	c.instanceCount = len(states)
	c.addons = make([]DisplayAddon, 0)
	if c.conf.AdvancedWall.InstanceHiding {
		c.addons = append(c.addons, &hider{})
	}
	if c.conf.AdvancedWall.PreviewFreezing {
		c.addons = append(c.addons, &freezer{}, &progressDisplay{})
	}
	for _, addon := range c.addons {
		if err := addon.Setup(conf, obs, states); err != nil {
			return err
		}
	}
	return nil
}

func (c *StandardController) Unlock(id int) error {
	c.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Lock %d", id+1), false)
	return nil
}

func (c *StandardController) Update(update mc.Update) error {
	for _, addon := range c.addons {
		addon.Update(update)
	}
	return nil
}

func (c *StandardController) UpdateProjector(width, height int) {
	c.instWidth = width / c.wallWidth
	c.instHeight = height / c.wallHeight
}
