package manager

import (
	"fmt"
	"strconv"

	"github.com/woofdoggo/resetti/mc"

	obs "github.com/woofdoggo/go-obs"
)

func setupObs(o *obs.Client, instances []mc.Instance) error {
	_, err := o.SetCurrentSceneCollection(
		fmt.Sprintf("resetti - %d multi", len(instances)),
	)
	if err != nil {
		return err
	}
	return setSources(o, instances)
}

func setSources(o *obs.Client, instances []mc.Instance) error {
	for i, v := range instances {
		srcSettings, err := o.GetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			"xcomposite_input",
		)
		if err != nil {
			return err
		}
		settings := srcSettings.SourceSettings.(map[string]interface{})
		settings["capture_window"] = strconv.Itoa(int(v.Window))
		_, err = o.SetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			"xcomposite_input",
			settings,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func getWallSize(o *obs.Client, count int) (uint16, uint16, error) {
	xs, ys := make([]float64, 0), make([]float64, 0)
	for i := 0; i < count; i++ {
		itemSettings, err := o.GetSceneItemProperties(
			"Wall",
			obs.GetSceneItemPropertiesItem{
				Name: fmt.Sprintf("MC %d", i+1),
			},
		)
		if err != nil {
			return 0, 0, err
		}
		xs = appendUnique(xs, itemSettings.Position.X)
		ys = appendUnique(ys, itemSettings.Position.Y)
	}
	return uint16(len(xs)), uint16(len(ys)), nil
}

func appendUnique(s []float64, i float64) []float64 {
	for _, v := range s {
		if i == v {
			return s
		}
	}
	return append(s, i)
}
