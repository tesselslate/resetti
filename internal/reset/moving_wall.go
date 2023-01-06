package reset

import (
	"fmt"
	"math"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type LockedView struct {
	width             int
	height            int
	instances         []mc.Instance
	obs               *obs.Client
	renderedInstances int
	lockQueue         []mc.Instance
}
type LoadingView struct {
	width             int
	height            int
	instances         []mc.Instance
	obs               *obs.Client
	renderedInstances int
	loadQueue         []mc.Instance
}
type FullView struct {
	width        int
	height       int
	lastAddedPos [2]int
	instances    []mc.Instance
	obs          *obs.Client
}

type MovingWall struct {
	lockedView  LockedView
	loadingView LoadingView
	fullView    FullView
	obs         *obs.Client
}

func DefaultMovingWall(obs *obs.Client, conf cfg.Profile) MovingWall {
	if !conf.MovingWall.UseMovingWall {
		return MovingWall{}
	}
	moving_wall := MovingWall{lockedView: LockedView{obs: obs}, loadingView: LoadingView{obs: obs}, fullView: FullView{obs: obs}, obs: obs}
	return moving_wall
}

func (m *MovingWall) SetupWallScene(conf cfg.Profile, instances []mc.Instance) error {
	canvas_width, canvas_height, err := m.obs.GetCanvasSize()
	if err != nil {
		return err
	}
	// Moving the scene sources in the wall scene to specified places.
	width, height := float64(canvas_width)/2, float64(canvas_height)/2
	err = m.obs.SetSceneItemTransform("Wall", "FullView", obs.Transform{X: 3 * width / 2, Y: height, Width: width / 2, Height: height, Bounds: "OBS_BOUNDS_STRETCH"})
	if err != nil {
		return err
	}
	err = m.obs.SetSceneItemTransform("Wall", "LoadingView", obs.Transform{X: 0, Y: 0, Width: float64(canvas_width), Height: height, Bounds: "OBS_BOUNDS_STRETCH"})
	if err != nil {
		return err
	}
	err = m.obs.SetSceneItemTransform("Wall", "LockedView", obs.Transform{X: 0, Y: height, Width: 3 * width / 2, Height: height, Bounds: "OBS_BOUNDS_STRETCH"})
	if err != nil {
		return err
	}

	// Setting up the views.
	m.fullView.width = canvas_width
	m.fullView.height = canvas_height
	m.fullView.instances = instances

	m.loadingView.width = canvas_width
	m.loadingView.height = canvas_height

	m.lockedView.width = canvas_width
	m.lockedView.height = canvas_height

	m.loadingView.renderedInstances = 0
	m.lockedView.renderedInstances = 0

	// Render the instances in correct places.
	err = m.render(instances)
	if err != nil {
		return err
	}
	return nil
}

func (m *FullView) renderInstance(instance mc.Instance, x int, y int, width int, height int, idx int) error {
	instance_name := fmt.Sprintf("MC %d FullView", idx+1)
	err := m.obs.SetSceneItemTransform("FullView", instance_name, obs.Transform{X: float64(x), Y: float64(y), Width: float64(width), Height: float64(height), Bounds: "OBS_BOUNDS_STRETCH"})
	if err != nil {
		return err
	}
	return nil
}

func (m *LockedView) renderInstance(instance mc.Instance) error {
	err := m.hideAll()
	if err != nil {
		return err
	}
	if m.renderedInstances < 4 {
		m.instances = append(m.instances, instance)
		m.renderedInstances++
	} else {
		m.lockQueue = append(m.lockQueue, instance)
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LockedView) unrenderInstance(instance mc.Instance) error {
	err := m.hideAll()
	if err != nil {
		return err
	}
	for i, inst := range m.instances {
		if inst.InstanceInfo.Id == instance.InstanceInfo.Id {
			copy(m.instances[i:], m.instances[i+1:])
			m.instances[len(m.instances)-1] = mc.Instance{}
			m.instances = m.instances[:len(m.instances)-1]
			break
		}
	}
	if len(m.lockQueue) != 0 {
		m.instances = append(m.instances, m.lockQueue[0])
		if len(m.lockQueue) != 1 {
			copy(m.lockQueue[0:], m.lockQueue[1:])
			m.lockQueue[len(m.lockQueue)-1] = mc.Instance{}
		}
		m.lockQueue = m.lockQueue[:len(m.lockQueue)-1]
	} else {
		m.renderedInstances--
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LockedView) update() error {
	sqrt := math.Ceil(math.Sqrt(float64(m.renderedInstances)))
	instWidth := float64(m.width) / (sqrt + 1)
	instHeight := float64(m.height) / (sqrt + 1)
	idx := 0
	for y := 0.0; y < float64(m.height); y += instHeight {
		for x := 0.0; x < float64(m.width); x += instWidth {
			if idx < m.renderedInstances {
				instName := fmt.Sprintf("MC %d LockedView", m.instances[idx].Id+1)
				err := m.obs.SetSceneItemVisible("LockedView", instName, true)
				if err != nil {
					return err
				}
				err = m.obs.SetSceneItemTransform("LockedView", instName, obs.Transform{X: x, Y: y, Width: instWidth, Height: instHeight, Bounds: "OBS_BOUNDS_STRETCH"})
				if err != nil {
					return err
				}
				idx++
			}
		}
	}
	return nil
}

func (m *LockedView) hideAll() error {
	for _, instance := range m.instances {
		instName := fmt.Sprintf("MC %d LockedView", instance.Id+1)
		err := m.obs.SetSceneItemVisible("LockedView", instName, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *LoadingView) renderInstance(instance mc.Instance) error {
	err := m.hideAll()
	if err != nil {
		return err
	}
	if m.renderedInstances < 4 {
		m.instances = append(m.instances, instance)
		m.renderedInstances++
	} else {
		m.loadQueue = append(m.loadQueue, instance)
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LoadingView) unrenderInstance(instance mc.Instance) error {
	err := m.hideAll()
	if err != nil {
		return err
	}
	for i, inst := range m.instances {
		if inst.InstanceInfo.Id == instance.InstanceInfo.Id {
			copy(m.instances[i:], m.instances[i+1:])
			m.instances[len(m.instances)-1] = mc.Instance{}
			m.instances = m.instances[:len(m.instances)-1]
			break
		}
	}
	if len(m.loadQueue) != 0 {
		m.instances = append(m.instances, m.loadQueue[0])
		if len(m.loadQueue) != 1 {
			copy(m.loadQueue[0:], m.loadQueue[1:])
			m.loadQueue[len(m.loadQueue)-1] = mc.Instance{}
		}
		m.loadQueue = m.loadQueue[:len(m.loadQueue)-1]
	} else {
		m.renderedInstances--
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LoadingView) update() error {
	sqrt := math.Ceil(math.Sqrt(float64(m.renderedInstances)))
	instWidth := float64(m.width) / (sqrt + 1)
	instHeight := float64(m.height) / (sqrt + 1)
	idx := 0
	for y := 0.0; y < float64(m.height); y += instHeight {
		for x := 0.0; x < float64(m.width); x += instWidth {
			if idx < m.renderedInstances {
				instName := fmt.Sprintf("MC %d LoadingView", m.instances[idx].Id+1)
				err := m.obs.SetSceneItemVisible("LoadingView", instName, true)
				if err != nil {
					return err
				}
				err = m.obs.SetSceneItemTransform("LoadingView", instName, obs.Transform{X: x, Y: y, Width: instWidth, Height: instHeight, Bounds: "OBS_BOUNDS_STRETCH"})
				if err != nil {
					return err
				}
				idx++
			}
		}
	}
	return nil
}

func (m *LoadingView) hideAll() error {
	for _, instance := range m.instances {
		instName := fmt.Sprintf("MC %d LoadingView", instance.Id+1)
		err := m.obs.SetSceneItemVisible("LoadingView", instName, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MovingWall) render(instances []mc.Instance) error {
	pos_x, pos_y := 0, 0
	divisor := math.Sqrt(float64(len(instances)))
	for i, instance := range instances {
		instName := fmt.Sprintf("MC %d LockedView", instance.Id+1)
		err := m.obs.SetSceneItemVisible("LockedView", instName, false)
		if err != nil {
			return err
		}
		if i < 4 {
			m.loadingView.instances = append(m.loadingView.instances, instance)
			m.loadingView.renderedInstances++
			err := m.loadingView.update()
			if err != nil {
				return err
			}
		} else {
			m.loadingView.loadQueue = append(m.loadingView.loadQueue, instance)
		}
	}
	width, height := m.fullView.width, m.fullView.height
	instWidth, instHeight := width/len(instances), height/len(instances)
	m.fullView.lastAddedPos = [2]int{pos_x, pos_y}
	for i, instance := range instances {
		err := m.fullView.renderInstance(instance, m.fullView.lastAddedPos[0], m.fullView.lastAddedPos[1], instWidth, instHeight, i)
		if err != nil {
			return err
		}
		if float64(i) >= divisor {
			m.fullView.lastAddedPos = [2]int{pos_x, pos_y + instHeight}
		} else {
			m.fullView.lastAddedPos = [2]int{pos_x + instWidth, pos_y}
		}
	}
	return nil
}
