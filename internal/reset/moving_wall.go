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
	width     int
	height    int
	instances []mc.Instance
	obs       *obs.Client
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
	flag := false
	for i, inst := range m.instances {
		if inst.InstanceInfo.Id == instance.InstanceInfo.Id {
			copy(m.instances[i:], m.instances[i+1:])
			m.instances[len(m.instances)-1] = mc.Instance{}
			m.instances = m.instances[:len(m.instances)-1]
			flag = true
			break
		}
	}
	if flag {
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
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LockedView) update() error {
	if m.renderedInstances == 0 {
		return nil
	}
	cols := int(math.Floor(math.Sqrt(float64(m.renderedInstances))))
	rows := int(math.Ceil(float64(m.renderedInstances) / float64(cols)))
	width, height := float64(m.width/cols), float64(m.height/rows)
	id := 0
	for y := 0; y < rows; y += 1 {
		for x := 0; x < cols; x += 1 {
			if id == m.renderedInstances {
				break
			}
			instName := fmt.Sprintf("MC %d LockedView", m.instances[id].Id+1)
			err := m.obs.SetSceneItemVisible("LockedView", instName, true)
			if err != nil {
				return err
			}
			err = m.obs.SetSceneItemTransform("LockedView", instName, obs.Transform{
				X:      float64(x) * width,
				Y:      float64(y) * height,
				Width:  width,
				Height: height,
				Bounds: "OBS_BOUNDS_STRETCH",
			})
			if err != nil {
				return err
			}
			id += 1
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
	flag := false
	for i, inst := range m.instances {
		if inst.InstanceInfo.Id == instance.InstanceInfo.Id {
			copy(m.instances[i:], m.instances[i+1:])
			m.instances[len(m.instances)-1] = mc.Instance{}
			m.instances = m.instances[:len(m.instances)-1]
			flag = true
			break
		}
	}
	if flag {
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
	}
	err = m.update()
	if err != nil {
		return err
	}
	return nil
}

func (m *LoadingView) update() error {
	if m.renderedInstances == 0 {
		return nil
	}
	width, height := float64(m.width)/2, float64(m.height)/2
	id := 0
	for y := 0; y < 2; y += 1 {
		for x := 0; x < 2; x += 1 {
			if id == m.renderedInstances {
				break
			}
			instName := fmt.Sprintf("MC %d LoadingView", m.instances[id].Id+1)
			err := m.obs.SetSceneItemVisible("LoadingView", instName, true)
			if err != nil {
				return err
			}
			err = m.obs.SetSceneItemTransform("LoadingView", instName, obs.Transform{
				X:      float64(x) * width,
				Y:      float64(y) * height,
				Width:  width,
				Height: height,
				Bounds: "OBS_BOUNDS_STRETCH",
			})
			if err != nil {
				return err
			}
			id += 1
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
	for i, instance := range instances {
		instName := fmt.Sprintf("MC %d LockedView", instance.Id+1)
		err := m.obs.SetSceneItemVisible("LockedView", instName, false)
		if err != nil {
			return err
		}
		instName = fmt.Sprintf("MC %d LoadingView", instance.Id+1)
		err = m.obs.SetSceneItemVisible("LoadingView", instName, false)
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
	cols := int(math.Floor(math.Sqrt(float64(len(instances)))))
	rows := int(math.Ceil(float64(len(instances)) / float64(cols)))
	width, height := m.fullView.width/cols, m.fullView.height/rows
	id := 0
	for y := 0; y < rows; y += 1 {
		for x := 0; x < cols; x += 1 {
			if id == len(instances) {
				break
			}
			err := m.fullView.renderInstance(instances[id], width*x, height*y, width, height, id)
			if err != nil {
				return err
			}
			id += 1
		}
	}
	return nil
}
