package reset

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// findProjector finds the OBS wall projector (if open.)
func findProjector(c *x11.Client) (xproto.Window, error) {
	windows, err := c.GetAllWindows()
	if err != nil {
		return 0, err
	}
	for _, win := range windows {
		title, err := c.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene) - Wall") {
			return win, nil
		}
	}
	return 0, errors.New("no projector")
}

// getWallSize returns the dimensions of the user's wall.
func getWallSize(o *obs.Client, instances int) (uint16, uint16, error) {
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
		transform, err := o.GetSceneItemTransform(
			"Wall",
			fmt.Sprintf("MC %d", i+1),
		)
		if err != nil {
			return 0, 0, err
		}
		xs = appendUnique(xs, transform.X)
		ys = appendUnique(ys, transform.Y)
	}
	return uint16(len(xs)), uint16(len(ys)), nil
}

// setSources sets the correct window captures for each Minecraft source.
func setSources(o *obs.Client, instances []Instance) error {
	for i, v := range instances {
		err := o.SetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			obs.StringMap{
				"capture_window": strconv.Itoa(int(v.Wid)),
			},
			true,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
