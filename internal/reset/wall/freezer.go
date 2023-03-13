package wall

import (
	"fmt"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// freezer freezes instance previews.
type freezer struct {
	conf   *cfg.Profile
	obs    *obs.Client
	states []mc.InstanceState
}

func (f *freezer) Setup(conf *cfg.Profile, o *obs.Client, states []mc.InstanceState) error {
	f.conf = conf
	f.obs = o
	f.states = make([]mc.InstanceState, len(states))
	copy(f.states, states)
	return nil
}

func (f *freezer) Update(update mc.Update) {
	prev := f.states[update.Id]
	next := update.State
	f.states[update.Id] = update.State
	nowDirt := next.State == mc.StDirt && prev.State != mc.StDirt
	if next.Progress >= f.conf.AdvancedWall.FreezeThreshold {
		go func() {
			<-time.After(10 * time.Millisecond)
			f.obs.SetSourceFilterEnabledAsync(
				fmt.Sprintf("Wall MC %d", update.Id+1),
				fmt.Sprintf("Freeze %d", update.Id+1),
				true,
			)
		}()
	}
	if nowDirt {
		f.obs.SetSourceFilterEnabledAsync(
			fmt.Sprintf("Wall MC %d", update.Id+1),
			fmt.Sprintf("Freeze %d", update.Id+1),
			false,
		)
	}
}
