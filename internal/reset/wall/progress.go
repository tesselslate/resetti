package wall

import (
	"fmt"
	"strconv"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// progressDisplay displays the world generation progress for each instance.
type progressDisplay struct {
	conf *cfg.Profile
	obs  *obs.Client

	states []mc.InstanceState
}

func (d *progressDisplay) Setup(conf *cfg.Profile, o *obs.Client, states []mc.InstanceState) error {
	d.conf = conf
	d.obs = o
	d.states = make([]mc.InstanceState, len(states))
	copy(d.states, states)
	return d.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(states); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Progress %d", i), false)
		}
		return nil
	})
}

func (d *progressDisplay) Update(update mc.Update) {
	prev := d.states[update.Id]
	next := update.State
	d.states[update.Id] = next
	if prev.State != mc.StPreview && next.State == mc.StPreview {
		d.obs.SetSceneItemVisibleAsync(
			"Wall",
			fmt.Sprintf("Progress %d", update.Id+1),
			true,
		)
		d.obs.SetSourceSettingsAsync(
			fmt.Sprintf("Progress %d", update.Id+1),
			obs.StringMap{"text": "0"},
			true,
		)
	} else if prev.State != mc.StIdle && next.State == mc.StIdle {
		d.obs.SetSceneItemVisibleAsync(
			"Wall",
			fmt.Sprintf("Progress %d", update.Id+1),
			false,
		)
	} else if prev.State != mc.StDirt && next.State == mc.StDirt {
		d.obs.SetSceneItemVisibleAsync(
			"Wall",
			fmt.Sprintf("Progress %d", update.Id+1),
			false,
		)
	}
	if next.State == mc.StPreview {
		d.obs.SetSourceSettingsAsync(
			fmt.Sprintf("Progress %d", update.Id+1),
			obs.StringMap{"text": strconv.Itoa(next.Progress)},
			true,
		)
	}
}
