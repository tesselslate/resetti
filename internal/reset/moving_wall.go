package reset

import (
	"fmt"
	"math"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type LockedView struct {
	width              int
	height             int
	totalInstanceCount int
	instances          []mc.Instance
	obs                *obs.Client
	renderedInstances  int
	lockQueue          []mc.Instance
}
type LoadingView struct {
	width              int
	height             int
	totalInstanceCount int
	instances          []mc.Instance
	obs                *obs.Client
	renderedInstances  int
	loadQueue          []mc.Instance
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
	err = m.obs.SetSceneItemBounds("Wall", "FullView", 3*width/2, height, width/2, height)
	if err != nil {
		return err
	}
	err = m.obs.SetSceneItemBounds("Wall", "LoadingView", 0, 0, float64(canvas_width), height)
	if err != nil {
		return err
	}
	err = m.obs.SetSceneItemBounds("Wall", "LockedView", 0, height, 3*width/2, height)
	if err != nil {
		return err
	}

	// Setting up the views.
	m.fullView.width = canvas_width
	m.fullView.height = canvas_height
	m.fullView.instances = instances

	m.loadingView.width = canvas_width
	m.loadingView.height = canvas_height
	m.loadingView.totalInstanceCount = len(instances)

	m.lockedView.width = canvas_width
	m.lockedView.height = canvas_height
	m.lockedView.totalInstanceCount = len(instances)

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
	err := m.obs.SetSceneItemBounds("FullView", instance_name, float64(x), float64(y), float64(width), float64(height))
	if err != nil {
		return err
	}
	return nil
}

func (m *LockedView) renderInstance(instance mc.Instance) error {
	if m.renderedInstances < 4 {
		m.instances = append(m.instances, instance)
		m.renderedInstances++
		err := m.update()
		if err != nil {
			return err
		}
	} else {
		m.lockQueue = append(m.lockQueue, instance)
	}
	return nil
}

func (m *LockedView) unrenderInstance(instance mc.Instance) error {
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
		err := m.update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *LockedView) update() error {
	if m.renderedInstances == 0 {
		// Hide all.
		return m.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
			for i := 1; i <= m.totalInstanceCount; i += 1 {
				instName := fmt.Sprintf("MC %d LockedView", i)
				b.SetItemVisibility("LockedView", instName, false)
			}
			return nil
		})
	}

	return m.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		cols := int(math.Floor(math.Sqrt(float64(m.renderedInstances))))
		rows := int(math.Ceil(float64(m.renderedInstances) / float64(cols)))
		width, height := float64(m.width/cols), float64(m.height/rows)
		id := 0

		// Hide all the instances.
		visible := make([]bool, m.totalInstanceCount)

		for _, instance := range m.instances {
			visible[instance.Id] = true
		}
		for i, visible := range visible {
			instName := fmt.Sprintf("MC %d LockedView", i+1)
			b.SetItemVisibility("LockedView", instName, visible)
		}

		// Show and position the correct instances.
		for y := 0; y < rows; y += 1 {
			for x := 0; x < cols; x += 1 {
				if id == m.renderedInstances {
					break
				}
				instName := fmt.Sprintf("MC %d LockedView", m.instances[id].Id+1)
				b.SetItemPosition("LockedView", instName,
					float64(x)*width,
					float64(y)*height,
					width,
					height,
				)
				id += 1
			}
		}
		return nil
	})
}

func (m *LoadingView) renderInstance(instance mc.Instance) error {
	if m.renderedInstances < 4 {
		m.instances = append(m.instances, instance)
		m.renderedInstances++
		err := m.update()
		if err != nil {
			return err
		}
	} else {
		m.loadQueue = append(m.loadQueue, instance)
	}
	return nil
}

func (m *LoadingView) unrenderInstance(instance mc.Instance) error {
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
		err := m.update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *LoadingView) update() error {
	if m.renderedInstances == 0 {
		return m.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
			for i := 1; i <= m.totalInstanceCount; i += 1 {
				instName := fmt.Sprintf("MC %d LoadingView", i)
				b.SetItemVisibility("LoadingView", instName, false)
			}
			return nil
		})
	}

	return m.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		width, height := float64(m.width)/2, float64(m.height)/2
		id := 0

		visible := make([]bool, m.totalInstanceCount)

		// Hide all instances.
		for _, instance := range m.instances {
			visible[instance.Id] = true
		}
		for i, visible := range visible {
			instName := fmt.Sprintf("MC %d LoadingView", i+1)
			b.SetItemVisibility("LoadingView", instName, visible)
		}

		// Show and position the correct instances.
		for y := 0; y < 2; y += 1 {
			for x := 0; x < 2; x += 1 {
				if id == m.renderedInstances {
					break
				}
				instName := fmt.Sprintf("MC %d LoadingView", m.instances[id].Id+1)
				b.SetItemPosition("LoadingView", instName,
					float64(x)*width,
					float64(y)*height,
					width,
					height,
				)
				id += 1
			}
		}
		return nil
	})
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
		} else {
			m.loadingView.loadQueue = append(m.loadingView.loadQueue, instance)
		}
	}
	err := m.loadingView.update()
	if err != nil {
		return errors.Wrap(err, "loading view")
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
