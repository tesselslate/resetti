package ctl

import (
	"fmt"
	"time"

	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/obs"
	"github.com/tesselslate/resetti/internal/x11"
	"golang.org/x/exp/slices"
)

// MovingWall implements a "moving wall" interface, where the user can see one
// or more groups of instances on an OBS projector and manage them from there.
type MovingWall struct {
	host *Controller
	conf *cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	instances  []mc.InstanceInfo // List of instance metadata
	states     []mc.State        // List of instance states
	queue      []int             // Instance queue. -1 marks empty.
	locks      []int             // List of locked instances.
	hitboxes   []hitbox          // Each instance's location on the projector.
	active     int               // Active instance. -1 is a sentinel for wall
	lastHitbox cfg.Rectangle     // The last hitbox a mouse action was done on.

	proj    ProjectorController
	freezer *freezer
	hider   *hider
}

// hitbox contains information about the state of an instance on the projector.
type hitbox struct {
	id        int // Instance ID
	box       cfg.Rectangle
	clickable bool
	z         int
}

// Setup implements Frontend.
func (m *MovingWall) Setup(deps frontendDependencies) error {
	m.host = deps.host
	m.conf = deps.conf
	m.obs = deps.obs
	m.x = deps.x

	m.active = -1
	m.lastHitbox = cfg.Rectangle{}
	m.instances = make([]mc.InstanceInfo, len(deps.instances))
	m.states = make([]mc.State, len(deps.states))
	copy(m.instances, deps.instances)
	copy(m.states, deps.states)
	if err := m.proj.Setup(deps.conf, deps.obs, deps.x); err != nil {
		return fmt.Errorf("projector setup: %w", err)
	}
	if err := prepareObs(deps.obs, deps.instances); err != nil {
		return fmt.Errorf("obs setup: %w", err)
	}
	if err := m.proj.Focus(); err != nil {
		return fmt.Errorf("focus projector: %w", err)
	}
	m.obs.SetScene("Wall")
	if m.conf.Wall.FreezeAt > 0 {
		m.freezer = newFreezer(deps.conf, deps.obs, deps.states)
	}
	if m.conf.Wall.ShowAt >= 0 {
		m.hider = newHider(deps.conf, deps.obs, deps.states)
	}

	// Fill the queue with all instances.
	for i := range deps.instances {
		m.queue = append(m.queue, i)
	}
	m.layout()
	return nil
}

// ProcessEvent implements Frontend.
func (m *MovingWall) ProcessEvent(evt x11.Event) {
	m.proj.ProcessEvent(evt)
}

// Input processes a single user input.
func (m *MovingWall) Input(input Input) {
	actions := m.conf.Keybinds[input.Bind]
	if m.active != -1 {
		if input.Held {
			return
		}
		for _, action := range actions.IngameActions {
			switch action.Type {
			case cfg.ActionIngameReset:
				if m.x.GetActiveWindow() == m.instances[m.active].Wid {
					m.resetIngame()
				}
			case cfg.ActionIngameFocus:
				m.host.FocusInstance(m.active)
			case cfg.ActionIngameRes:
				m.host.ToggleResolution(m.active)
			}
		}
	} else {
		for _, action := range actions.WallActions {
			// wall_focus_projector is the only wall action that can be taken
			// while the projector isn't focused.
			if action.Type == cfg.ActionWallFocus {
				if input.Held {
					continue
				}
				if err := m.proj.Focus(); err != nil {
					log.Error("Input: Failed to focus projector: %s", err)
				}
			}
			if m.active != -1 || !m.proj.Active {
				continue
			}

			switch action.Type {
			case cfg.ActionWallResetAll:
				if input.Held {
					continue
				}
				m.wallResetAll()
				m.collapseEmpty()
				m.layout()
			case cfg.ActionWallPlayFirstLocked:
				if input.Held {
					continue
				}
				if m.conf.Wall.Moving.ResetBeforePlay {
					g := m.conf.Wall.Moving.Groups[0]
					if len(m.queue) > int(g.Width*g.Height) {
						continue
					}
				}
				m.playFirstLocked()
				m.collapseEmpty()
			case cfg.ActionWallLock, cfg.ActionWallPlay, cfg.ActionWallReset, cfg.ActionWallResetOthers:
				var id int
				if action.Extra != nil {
					if input.Held {
						continue
					}
					id = *action.Extra
					if id >= len(m.queue) {
						continue
					}
					id = m.queue[id]
				} else {
					// Only accept mouse-based inputs if the projector is focused.
					if !m.proj.Active {
						continue
					}
					// Ungrab the pointer if the user clicks outside of
					// the projector.
					if !m.proj.InBounds(input.X, input.Y) {
						m.proj.Unfocus()
						continue
					}
					hitbox, ok := m.getHitbox(input)
					if !ok {
						continue
					}
					if input.Held && hitbox.box == m.lastHitbox {
						continue
					}
					id = hitbox.id
					m.lastHitbox = hitbox.box
				}
				if id < 0 || id > len(m.instances)-1 {
					continue
				}
				switch action.Type {
				case cfg.ActionWallLock:
					m.wallLock(id)
				case cfg.ActionWallPlay:
					if m.states[id].Type == mc.StIdle {
						if m.conf.Wall.Moving.ResetBeforePlay {
							g := m.conf.Wall.Moving.Groups[0]
							if len(m.queue) > int(g.Width*g.Height) {
								continue
							}
						}
						m.wallPlay(id)
						m.collapseEmpty()
					}
				case cfg.ActionWallReset:
					m.wallReset(id)
					m.collapseEmpty()
				case cfg.ActionWallResetOthers:
					if m.states[id].Type == mc.StIdle {
						m.wallResetOthers(id)
						m.collapseEmpty()
					}
				}
				m.layout()
			}
		}
	}
}

// Update processes a single instance state update.
func (m *MovingWall) Update(update mc.Update) {
	if m.freezer != nil {
		m.freezer.Update(update)
	}
	if m.hider != nil {
		if m.hider.ShouldShow(update) {
			if !slices.Contains(m.queue, update.Id) {
				m.queue = append(m.queue, update.Id)
				m.layout()
			}
		}
	} else {
		prev := m.states[update.Id].Type
		next := update.State.Type
		if !slices.Contains(m.queue, update.Id) && !slices.Contains(m.locks, update.Id) {
			nowPreview := prev != mc.StPreview && next == mc.StPreview
			catchIdle := next == mc.StIdle
			if nowPreview || catchIdle {
				m.queue = append(m.queue, update.Id)
				m.layout()
			}
		}
	}
	m.states[update.Id] = update.State
}

// collapseEmpty removes all empty instances from the queue.
func (m *MovingWall) collapseEmpty() {
	var newQueue []int
	for _, id := range m.queue {
		if id != -1 {
			newQueue = append(newQueue, id)
		}
	}
	m.queue = newQueue
}

// getHitbox gets the hitbox the given input intersects with, if any.
func (m *MovingWall) getHitbox(input Input) (hitbox, bool) {
	x, y := m.proj.ToVideo(input.X, input.Y)
	var hits []hitbox
	for _, hitbox := range m.hitboxes {
		a := x >= int(hitbox.box.X) && x <= int(hitbox.box.X)+int(hitbox.box.W)
		b := y >= int(hitbox.box.Y) && y <= int(hitbox.box.Y)+int(hitbox.box.H)
		if a && b && hitbox.clickable {
			hits = append(hits, hitbox)
		}
	}
	switch len(hits) {
	case 0:
		return hitbox{}, false
	case 1:
		return hits[0], true
	default:
		slices.SortFunc(hits, func(a, b hitbox) bool {
			return a.z < b.z
		})
		return hits[0], true
	}
}

// layout updates the mapping of hitboxes to instance IDs and rerenders the
// wall scene based on the current state of the queue and locked group.
func (m *MovingWall) layout() {
	m.hitboxes = make([]hitbox, 0, len(m.instances))
	groups := m.conf.Wall.Moving.Groups

	from := 0
	for _, group := range groups {
		if from == len(m.queue) {
			break
		}
		to := from + int(group.Width*group.Height)
		if to > len(m.queue) {
			to = len(m.queue)
		}
		m.layoutGroup(group, from, m.queue[from:to])
		from = to
	}
	if m.conf.Wall.Moving.Locks != nil {
		m.layoutGroup(*m.conf.Wall.Moving.Locks, from, m.locks)
	}

	m.layoutObs()
}

// layoutGroup updates the layout of a specific group of instances.
func (m *MovingWall) layoutGroup(group cfg.Group, startZ int, instances []int) {
	instWidth := group.Space.W / group.Width
	instHeight := group.Space.H / group.Height
	for idx, inst := range instances {
		if inst == -1 {
			continue
		}
		x := uint32(idx) % group.Width
		y := uint32(idx) / group.Width
		if y >= group.Height {
			break
		}
		box := cfg.Rectangle{
			X: group.Space.X + int32(x*instWidth),
			Y: group.Space.Y + int32(y*instHeight),
			W: instWidth,
			H: instHeight,
		}
		hitbox := hitbox{
			inst,
			box,
			!group.Cosmetic,
			startZ + idx,
		}
		m.hitboxes = append(m.hitboxes, hitbox)
	}
}

// layoutObs adjusts the position of each instance on the wall scene according
// to their positions in queue or in the locked group.
func (m *MovingWall) layoutObs() {
	m.obs.BatchAsync(obs.SerialFrame, func(b *obs.Batch) {
		visible := make([]bool, len(m.instances))
		for i := len(m.hitboxes) - 1; i >= 0; i -= 1 {
			h := m.hitboxes[i]
			visible[h.id] = true
			name := fmt.Sprintf("Wall MC %d", h.id+1)
			b.SetItemIndex("Wall", name, 0)
			b.SetItemBounds(
				"Wall",
				name,
				float64(h.box.X),
				float64(h.box.Y),
				float64(h.box.W),
				float64(h.box.H),
			)
		}
		for i, visible := range visible {
			if visible {
				continue
			}
			name := fmt.Sprintf("Wall MC %d", i+1)
			b.SetItemBounds(
				"Wall",
				name,
				float64(m.proj.BaseWidth), float64(m.proj.BaseHeight), 1, 1,
			)
		}
	})
}

// playFirstLocked plays the first idle, locked instance. If no instance fits
// the criteria, it returns false. Otherwise, it returns true.
func (m *MovingWall) playFirstLocked() bool {
	for _, id := range m.locks {
		if m.states[id].Type == mc.StIdle {
			m.wallPlay(id)
			return true
		}
	}
	return false
}

// removeFromQueue removes the given instance from the queue if it is in it.
func (m *MovingWall) removeFromQueue(id int) {
	idx := slices.Index(m.queue, id)
	if idx != -1 {
		m.queue = slices.Delete(m.queue, idx, idx+1)
	}
}

// resetIngame resets the active instance.
func (m *MovingWall) resetIngame() {
	m.host.ResetInstance(m.active)
	m.host.RunHook(HookReset)
	if m.freezer != nil {
		m.freezer.Unfreeze(m.active)
	}
	m.active = -1
	if m.conf.Wall.GotoLocked && m.playFirstLocked() {
		return
	}
	m.layout()
	if err := m.proj.Focus(); err != nil {
		log.Error("resetIngame: Failed to focus projector: %s", err)
	}
	m.host.DeleteSleepbgLock(false)
	m.obs.SetScene("Wall")
}

// setLocked sets the lock state of the given instance.
func (m *MovingWall) setLocked(id int, lock bool) {
	idx := slices.Index(m.locks, id)
	if (idx != -1) == lock {
		return
	}
	if lock {
		m.locks = append(m.locks, id)
	} else {
		m.locks = slices.Delete(m.locks, idx, idx+1)
		m.queue = append(m.queue, id)
	}
	m.host.SetPriority(id, lock)
}

// wallLock toggles the lock state of the given instance.
func (m *MovingWall) wallLock(id int) {
	lock := !slices.Contains(m.locks, id)
	if m.freezer != nil {
		m.freezer.SetCanFreeze(id, !lock)
	}
	m.setLocked(id, lock)
	if lock {
		idx := slices.Index(m.queue, id)
		m.queue[idx] = -1
		m.host.RunHook(HookLock)
	} else {
		m.host.RunHook(HookUnlock)
		if m.conf.Wall.ResetUnlock {
			m.wallReset(id)
		}
	}
	if !m.conf.Wall.Moving.Gaps {
		m.collapseEmpty()
	}
}

// wallPlay plays the given instance. It is the caller's responsibility to check
// if the instance is in a valid state for playing.
func (m *MovingWall) wallPlay(id int) {
	m.active = id
	m.proj.Unfocus()
	m.host.PlayInstance(id)
	m.host.RunHook(HookWallPlay)
	m.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(m.instances); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == id)
		}
	})
	m.obs.SetScene("Instance")
	m.setLocked(id, false)
	m.removeFromQueue(id)
	m.host.CreateSleepbgLock()
}

// wallReset resets the given instance.
func (m *MovingWall) wallReset(id int) {
	if slices.Contains(m.locks, id) || m.states[id].Type == mc.StIngame {
		return
	}
	m.removeFromQueue(id)
	if m.host.ResetInstance(id) {
		if m.freezer != nil {
			m.freezer.Unfreeze(id)
		}
		m.host.RunHook(HookWallReset)
	} else {
		m.queue = append(m.queue, id)
	}
}

// wallResetAll resets all instances in the first group.
func (m *MovingWall) wallResetAll() {
	start := time.Now()
	group := m.conf.Wall.Moving.Groups[0]
	to := int(group.Width * group.Height)
	if to > len(m.queue) {
		to = len(m.queue)
	}
	for i := to - 1; i >= 0; i -= 1 {
		if m.queue[i] != -1 {
			m.wallReset(m.queue[i])
		}
	}

	log.Info("Reset all in %.2f ms", float64(time.Since(start).Microseconds())/1000)
}

// wallResetOthers plays an instance and resets all others in the first group.
// It is the caller's responsibility to check that the instance is in a valid
// state for playing.
func (m *MovingWall) wallResetOthers(id int) {
	group := m.conf.Wall.Moving.Groups[0]
	to := int(group.Width * group.Height)
	if to > len(m.queue) {
		to = len(m.queue)
	}
	for i := to - 1; i >= 0; i -= 1 {
		if m.queue[i] != -1 && m.queue[i] != id {
			m.wallReset(m.queue[i])
		}
	}
	m.wallPlay(id)
}
