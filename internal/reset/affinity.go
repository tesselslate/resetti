package reset

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
)

// Affinity states
const (
	// affIdle is used when the instance is finished generating.
	affIdle int = iota

	// affLow is used when the instance is past the user's `low_threshold`.
	affLow

	// affMid is used when the instance would be high priority but the user is
	// playing an instance.
	affMid

	// affHigh is used when the instance has not yet reached the `low_threshold`.
	affHigh

	// affActive is used for the currently active instance.
	affActive
)

//go:embed scripts/cgroup_setup.sh
var cgroup_script []byte

// CpuManager manages the CPU affinity of instances to improve performance.
type CpuManager struct {
	*sync.Mutex

	conf       cfg.Profile
	instances  []mc.InstanceInfo
	states     []mc.InstanceState
	prioritize []bool   // Which instances to prioritize.
	groups     []int    // Which affinity group each instance is in.
	burst      chan int // Move instances out of the burst group (mid).
	anyActive  bool     // Whether any instances are ingame.
}

// NewCpuManager creates a new CpuManager with the given configuration profile
// and instance list.
func NewCpuManager(conf cfg.Profile, instances []mc.InstanceInfo) (CpuManager, error) {
	if err := runCgroupScript(&conf); err != nil {
		return CpuManager{}, err
	}
	m := CpuManager{
		&sync.Mutex{},
		conf,
		instances,
		make([]mc.InstanceState, len(instances)),
		make([]bool, len(instances)),
		make([]int, len(instances)),
		make(chan int, len(instances)),
		false,
	}
	if conf.AdvancedWall.BurstLength > 0 {
		go m.handleBurst()
	}
	return m, m.writeCpuSets()
}

// SetPriority prioritizes or deprioritizes the given instance.
func (m *CpuManager) SetPriority(id int, prioritize bool) error {
	m.Lock()
	defer m.Unlock()
	m.prioritize[id] = prioritize
	return m.updateAffinity(id)
}

// Update changes the state of the instance and updates its affinity group.
func (m *CpuManager) Update(id int, state mc.InstanceState) error {
	m.Lock()
	defer m.Unlock()
	activeChanged := false
	if state.State == mc.StIngame {
		m.anyActive = true
		activeChanged = true
	} else if m.states[id].State == mc.StIngame && state.State != mc.StIngame {
		m.anyActive = false
		activeChanged = true
	}
	m.states[id] = state
	if activeChanged {
		return m.updateAffinities()
	} else {
		return m.updateAffinity(id)
	}
}

// handleBurst runs in the background and handles moving instances out of mid
// affinity and back to idle.
func (m *CpuManager) handleBurst() {
	for id := range m.burst {
		m.Lock()
		if m.states[id].State == mc.StIdle {
			if err := m.moveInstance(id, affIdle); err != nil {
				log.Printf("Burst handling failure: %s\n", err)
			}
		}
		m.Unlock()
	}
}

// moveInstance moves the instance to the given cgroup.
func (m *CpuManager) moveInstance(id int, affinity int) error {
	if m.groups[id] == affinity {
		return nil
	}
	m.groups[id] = affinity
	group := []string{"idle", "low", "mid", "high", "active"}[affinity]
	if m.conf.AdvancedWall.CcxSplit {
		if id < len(m.instances)/2 {
			group += "0"
		} else {
			group += "1"
		}
	}
	return os.WriteFile(
		fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cgroup.procs", group),
		[]byte(strconv.Itoa(int(m.instances[id].Pid))),
		0644,
	)
}

// updateAffinites updates the affinity of all instances.
func (m *CpuManager) updateAffinities() error {
	for i := 0; i < len(m.instances); i += 1 {
		if err := m.updateAffinity(i); err != nil {
			return err
		}
	}
	return nil
}

// updateAffinity updates the affinity of an individual instance.
func (m *CpuManager) updateAffinity(id int) error {
	switch m.states[id].State {
	case mc.StMenu:
		return nil
	case mc.StIdle:
		if m.conf.AdvancedWall.BurstLength > 0 {
			go func() {
				<-time.After(time.Duration(m.conf.AdvancedWall.BurstLength) * time.Millisecond)
				m.burst <- id
			}()
			return m.moveInstance(id, affMid)
		} else {
			return m.moveInstance(id, affIdle)
		}
	case mc.StDirt:
		if !m.anyActive {
			return m.moveInstance(id, affHigh)
		} else {
			return m.moveInstance(id, affMid)
		}
	case mc.StPreview:
		if !m.prioritize[id] && m.states[id].Progress > m.conf.AdvancedWall.LowThreshold {
			return m.moveInstance(id, affLow)
		}
		if !m.anyActive {
			return m.moveInstance(id, affHigh)
		} else {
			return m.moveInstance(id, affMid)
		}
	case mc.StIngame:
		return m.moveInstance(id, affActive)
	}
	panic(fmt.Sprintf("unreachable (%d)", m.states[id].State))
}

// writeCpuSet modifies the CPUs assigned to a given cgroup.
func (m *CpuManager) writeCpuSet(cgroup string, set []int) error {
	list := make([]string, 0, len(set))
	for _, cpu := range set {
		list = append(list, strconv.Itoa(cpu))
	}
	return os.WriteFile(
		fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cpuset.cpus", cgroup),
		[]byte(strings.Join(list, ",")),
		0644,
	)
}

// writeCpuSets writes the CPU sets for each necessary cgroup.
func (m *CpuManager) writeCpuSets() error {
	baseGroups := []string{"idle", "low", "mid", "high", "active"}
	cpus := []int{
		m.conf.AdvancedWall.CpusIdle,
		m.conf.AdvancedWall.CpusLow,
		m.conf.AdvancedWall.CpusMid,
		m.conf.AdvancedWall.CpusHigh,
		m.conf.AdvancedWall.CpusActive,
	}

	for i, group := range baseGroups {
		set := make([]int, cpus[i])
		for cpu := 0; cpu < cpus[i]; cpu += 1 {
			set = append(set, cpu*2)
		}
		if !m.conf.AdvancedWall.CcxSplit {
			if err := m.writeCpuSet(group, set); err != nil {
				return err
			}
		} else {
			if err := m.writeCpuSet(group+"0", set); err != nil {
				return err
			}
			for i := range set {
				set[i] += 1
			}
			if err := m.writeCpuSet(group+"1", set); err != nil {
				return err
			}
		}
	}
	return nil
}

// runCgroupScript runs the cgroup setup script.
func runCgroupScript(conf *cfg.Profile) error {
	// Check if the script needs to be run. Start by making sure the cgroup
	// folders exist.
	baseGroups := []string{
		"idle",
		"low",
		"mid",
		"high",
		"active",
	}
	var checkFolders []string
	if !conf.AdvancedWall.CcxSplit {
		checkFolders = baseGroups
	} else {
		checkFolders = make([]string, 0, len(baseGroups)*2)
		for _, v := range baseGroups {
			checkFolders = append(checkFolders, v+"0")
			checkFolders = append(checkFolders, v+"1")
		}
	}
	needsRun := false
	for _, folder := range checkFolders {
		stat, err := os.Stat("/sys/fs/cgroup/resetti/" + folder)
		if err != nil || !stat.IsDir() {
			needsRun = true
			break
		}
	}
	if !needsRun {
		log.Println("Skipped cgroup script.")
		return nil
	}

	// Check for the script's existence.
	path, err := cfg.GetFolder()
	if err != nil {
		return errors.Wrap(err, "get config folder")
	}
	path += "/cgroup_setup.sh"
	if err = os.WriteFile(path, cgroup_script, 0644); err != nil {
		return errors.Wrap(err, "write cgroup script")
	}

	// Determine the user's suid binary.
	suidBin, ok := os.LookupEnv("RESETTI_SUID_BINARY")
	if !ok {
		// TODO: More? pkexec, etc
		options := []string{"sudo", "doas"}
		for _, option := range options {
			cmd := exec.Command(option)
			err = cmd.Run()
			if !errors.Is(err, exec.ErrNotFound) {
				suidBin = option
				break
			}
		}
	}
	if suidBin == "" {
		return errors.Wrap(err, "no suid binary found")
	}

	// Run the script.
	subgroups := strings.Join(checkFolders, " ")
	cmd := exec.Command(suidBin, "sh", path, subgroups)
	return errors.Wrap(cmd.Run(), "run cgroup script")
}
