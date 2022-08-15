package reset

import (
	"fmt"
	"strconv"

	go_obs "github.com/woofdoggo/go-obs"
)

// getWallSize returns the dimensions of the user's wall.
func getWallSize(o *go_obs.Client, instances int) (int, int, error) {
	appendUnique := func(slice []float64, item float64) []float64 {
		for _, v := range slice {
			if item == v {
				return slice
			}
		}
		return append(slice, item)
	}
	xs, ys := make([]float64, 0), make([]float64, 0)
	for i := 0; i < instances; i++ {
		settings, err := o.GetSceneItemProperties(
			"Wall",
			go_obs.GetSceneItemPropertiesItem{
				Name: fmt.Sprintf("MC %d", i+1),
			},
		)
		if err != nil {
			return 0, 0, err
		}
		xs = appendUnique(xs, settings.Position.X)
		ys = appendUnique(ys, settings.Position.Y)
	}
	return len(xs), len(ys), nil
}

// setScene sets the current OBS scene.
func setScene(o *go_obs.Client, name string) error {
	if o == nil {
		return nil
	}
	_, err := o.SetCurrentScene(name)
	return err
}

// setSceneCollection sets the current OBS scene collection.
func setSceneCollection(o *go_obs.Client, name string) error {
	_, err := o.SetCurrentSceneCollection(name)
	return err
}

// setSources sets the correct window captures for each Minecraft source.
func setSources(o *go_obs.Client, instances []Instance) error {
	for i, v := range instances {
		opts, err := o.GetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			"xcomposite_input",
		)
		if err != nil {
			return err
		}
		settings := opts.SourceSettings.(map[string]interface{})
		settings["capture_window"] = strconv.Itoa(int(v.Wid))
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

// setVisible adjusts the visibility of a scene item.
func setVisible(o *go_obs.Client, scene string, item string, visible bool) error {
	props, err := o.GetSceneItemProperties(
		scene,
		go_obs.GetSceneItemPropertiesItem{
			Name: item,
		},
	)
	if err != nil {
		return err
	}
	yes := true
	_, err = o.SetSceneItemProperties(
		scene,
		go_obs.SetSceneItemPropertiesItem{
			Name: item,
		},
		go_obs.SetSceneItemPropertiesPosition{
			X:         &props.Position.X,
			Y:         &props.Position.Y,
			Alignment: &props.Position.Alignment,
		},
		&props.Rotation,
		go_obs.SetSceneItemPropertiesScale{
			X:      &props.Scale.X,
			Y:      &props.Scale.Y,
			Filter: props.Scale.Filter,
		},
		go_obs.SetSceneItemPropertiesCrop{
			Top:    &props.Crop.Top,
			Right:  &props.Crop.Right,
			Bottom: &props.Crop.Bottom,
			Left:   &props.Crop.Left,
		},
		&visible,
		&yes,
		go_obs.SetSceneItemPropertiesBounds{
			Type:      props.Bounds.Type,
			Alignment: &props.Bounds.Alignment,
			X:         &props.Bounds.X,
			Y:         &props.Bounds.Y,
		},
	)
	return err
}
