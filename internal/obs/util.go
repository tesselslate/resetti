package obs

import (
	"fmt"
	"strconv"

	"github.com/woofdoggo/resetti/internal/mc"
)

func appendUnique(s []float64, i float64) []float64 {
	for _, v := range s {
		if i == v {
			return s
		}
	}
	return append(s, i)
}

func setMcSources(instances []mc.Instance) error {
	for i, v := range instances {
		srcSettings, err := client.GetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			"xcomposite_input",
		)
		if err != nil {
			return err
		}
		settings := srcSettings.SourceSettings.(map[string]interface{})
		settings["capture_window"] = strconv.Itoa(int(v.Window))
		_, err = client.SetSourceSettings(
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
