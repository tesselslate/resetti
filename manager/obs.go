package manager

import (
	"fmt"
	"resetti/mc"
	"strconv"

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
