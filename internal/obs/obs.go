// Package obs provides a single, shared OBS websocket client.
package obs

import (
	"fmt"

	go_obs "github.com/woofdoggo/go-obs"
	"github.com/woofdoggo/resetti/internal/mc"
)

var client *go_obs.Client

func Initialize(port uint16, password string) (chan error, error) {
	client = &go_obs.Client{}
	authReq, errch, err := client.Connect(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return errch, err
	}
	if !authReq {
		return errch, nil
	}
	err = client.Login(password)
	return errch, err
}

func GetWallSize(numInstances int) (uint16, uint16, error) {
	xs, ys := make([]float64, 0), make([]float64, 0)
	for i := 0; i < numInstances; i++ {
		itemSettings, err := client.GetSceneItemProperties(
			"Wall",
			go_obs.GetSceneItemPropertiesItem{
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

func OpenProjector() error {
	_, err := client.OpenProjector("Scene", nil, "", "Wall")
	return err
}

func SetScene(name string) error {
	_, err := client.SetCurrentScene(name)
	return err
}

func SetVisible(scene string, item string, visible bool) error {
	res, err := client.GetSceneItemProperties(
		scene,
		go_obs.GetSceneItemPropertiesItem{
			Name: item,
		},
	)
	if err != nil {
		return err
	}
	yes := true
	_, err = client.SetSceneItemProperties(
		scene,
		go_obs.SetSceneItemPropertiesItem{
			Name: item,
		},
		go_obs.SetSceneItemPropertiesPosition{
			X:         &res.Position.X,
			Y:         &res.Position.Y,
			Alignment: &res.Position.Alignment,
		},
		&res.Rotation,
		go_obs.SetSceneItemPropertiesScale{
			X:      &res.Scale.X,
			Y:      &res.Scale.Y,
			Filter: res.Scale.Filter,
		},
		go_obs.SetSceneItemPropertiesCrop{
			Top:    &res.Crop.Top,
			Right:  &res.Crop.Right,
			Bottom: &res.Crop.Bottom,
			Left:   &res.Crop.Left,
		},
		&visible,
		&yes,
		go_obs.SetSceneItemPropertiesBounds{
			Type:      res.Bounds.Type,
			Alignment: &res.Bounds.Alignment,
			X:         &res.Bounds.X,
			Y:         &res.Bounds.Y,
		},
	)
	return err
}

func SetupScenes(instances []mc.Instance) error {
	_, err := client.SetCurrentSceneCollection(
		fmt.Sprintf("resetti - %d multi", len(instances)),
	)
	if err != nil {
		return err
	}
	return setMcSources(instances)
}
