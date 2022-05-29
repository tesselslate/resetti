package setup

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	obs "github.com/woofdoggo/go-obs"
)

type choices struct {
	Instances       uint
	Wall            bool
	WallX           uint
	WallY           uint
	InstancesAlways bool
}

func CmdObs() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Before continuing, please setup your OBS websocket on")
	fmt.Println("port 4440 with authentication disabled.")
	fmt.Println("If the process fails, then delete the scene collection")
	fmt.Println("and try again.")
	fmt.Println("Once ready, press enter.")
	_, _ = reader.ReadString('\n')

	info := choices{}
	fmt.Println("How many instances are you using?")
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Try again.")
			continue
		}
		num, err := strconv.Atoi(strings.Trim(input, "\n"))
		if err != nil {
			fmt.Println("Try again.")
			continue
		}
		if num < 1 {
			fmt.Println("Enter a positive non-zero number.")
			continue
		}
		info.Instances = uint(num)
		break
	}
	fmt.Println("Do you want to create a Wall scene? (y/n)")
outer:
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Try again.")
			continue
		}
		if len(input) == 0 {
			fmt.Println("Try again.")
			continue
		}
		switch unicode.ToLower(rune(input[0])) {
		case 'y':
			info.Wall = true
			break outer
		case 'n':
			info.Wall = false
			break outer
		default:
			fmt.Println("Try again.")
			continue
		}
	}
	if !info.Wall {
		fmt.Println("Please create a new scene collection with the name:")
		fmt.Printf("resetti - %d multi\n", info.Instances)
		fmt.Println("Once done, press enter. Then resetti will create the")
		fmt.Println("necessary scenes for you automatically.")
		reader.ReadString('\n')
		createScenes(info)
	}
	fmt.Println("How do you want your wall scene setup?")
	fmt.Println("Please enter the number of rows and columns you want.")
	fmt.Println("(e.g., '2x3' for 2 columns and 3 rows)")
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Try again.")
			continue
		}
		splits := strings.Split(strings.Trim(input, "\n"), "x")
		if len(splits) != 2 {
			fmt.Println("Try again.")
			continue
		}
		x, err := strconv.Atoi(splits[0])
		if err != nil {
			fmt.Println("Failed to convert rows. Try again.")
			continue
		}
		if x < 1 {
			fmt.Println("Enter a positive non-zero number.")
			continue
		}
		y, err := strconv.Atoi(splits[1])
		if err != nil {
			fmt.Println("Failed to convert columns. Try again.")
			continue
		}
		if y < 1 {
			fmt.Println("Enter a positive non-zero number.")
			continue
		}
		info.WallX = uint(x)
		info.WallY = uint(y)
		break
	}
	fmt.Println("Would you like your instances to be visible in OBS, even")
	fmt.Println("during gameplay? You probably do not want this if you are")
	fmt.Println("using Source Record or 2 OBS instances. (y/n)")
outer2:
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Try again.")
			continue
		}
		if len(input) == 0 {
			fmt.Println("Try again.")
			continue
		}
		switch unicode.ToLower(rune(input[0])) {
		case 'y':
			info.InstancesAlways = true
			break outer2
		case 'n':
			info.InstancesAlways = false
			break outer2
		default:
			fmt.Println("Try again.")
			continue
		}
	}
	fmt.Println("Please create a new scene collection with the name:")
	fmt.Printf("resetti - %d multi\n", info.Instances)
	fmt.Println("Once done, press enter. Then resetti will create the")
	fmt.Println("necessary scenes for you automatically.")
	reader.ReadString('\n')
	createScenes(info)
}

func getCanvasSize(o *obs.Client) (uint, uint, error) {
	res, err := o.GetVideoInfo()
	if err != nil {
		return 0, 0, err
	}
	return uint(res.BaseWidth), uint(res.BaseHeight), nil
}

func createScenes(info choices) {
	o := &obs.Client{}
	auth, _, err := o.Connect("localhost:4440")
	if err != nil {
		fmt.Println("Failed to connect to OBS:", err)
		return
	}
	if auth {
		fmt.Println("Disable password authentication in your OBS websocket settings.")
		return
	}
	fmt.Print("\n\n")
	// Set scene collection.
	fmt.Println("Setting scene collection.")
	_, err = o.SetCurrentSceneCollection(
		fmt.Sprintf("resetti - %d multi", info.Instances),
	)
	if err != nil {
		fmt.Println("Failed to set scene collection:", err)
		return
	}
	// Create scenes.
	fmt.Println("Creating instance scenes.")
	for i := uint(0); i < info.Instances; i++ {
		_, err := o.CreateScene(
			fmt.Sprintf("Instance %d", i+1),
		)
		if err != nil {
			fmt.Println("Failed to create scene:", err)
			return
		}
	}
	if info.Wall {
		fmt.Println("Creating wall scene.")
		_, err := o.CreateScene("Wall")
		if err != nil {
			fmt.Println("Failed to create scene:", err)
			return
		}
	}
	// Create instance sources.
	fmt.Println("Creating Minecraft sources.")
	for i := uint(0); i < info.Instances; i++ {
		_, err := o.CreateSource(
			fmt.Sprintf("MC %d", i+1),
			"xcomposite_input",
			"Wall",
			nil,
			ptr(true),
		)
		if err != nil {
			fmt.Println("Failed to create source:", err)
			return
		}
	}
	// Populate scene for each instance.
	fmt.Println("Getting OBS canvas size.")
	cw, ch, err := getCanvasSize(o)
	if err != nil {
		fmt.Println("Failed to get canvas size:", err)
		return
	}
	for i := uint(0); i < info.Instances; i++ {
		fmt.Printf("Populating Instance %d.\n", i+1)
		scene := fmt.Sprintf("Instance %d", i+1)
		source := fmt.Sprintf("MC %d", i+1)
		res, err := o.AddSceneItem(
			scene,
			source,
			ptr(true),
		)
		if err != nil {
			fmt.Println("Failed to create scene item:", err)
			return
		}
		_, err = o.SetSceneItemProperties(
			scene,
			obs.SetSceneItemPropertiesItem{
				Id: ptr(res.ItemId),
			},
			obs.SetSceneItemPropertiesPosition{
				X: ptr(0.0),
				Y: ptr(0.0),
			},
			nil,
			obs.SetSceneItemPropertiesScale{
			},
			obs.SetSceneItemPropertiesCrop{},
			ptr(true),
			ptr(true),
			obs.SetSceneItemPropertiesBounds{
				X: ptr(float64(cw)),
				Y: ptr(float64(ch)),
			},
		)
		if err != nil {
			fmt.Println("Failed to set scene item properties:", err)
			return
		}
		if info.InstancesAlways {
			x := cw - (cw / 8)
			y := ch - (ch / 10)
			for j := uint(0); j < info.Instances; j++ {
				if i == j {
					continue
				}
				source := fmt.Sprintf("MC %d", j+1)
				res, err := o.AddSceneItem(
					scene,
					source,
					ptr(true),
				)
				if err != nil {
					fmt.Println("Failed to create scene item:", err)
					return
				}
				_, err = o.SetSceneItemProperties(
					scene,
					obs.SetSceneItemPropertiesItem{
						Id: ptr(res.ItemId),
					},
					obs.SetSceneItemPropertiesPosition{
						X: ptr(float64(x)),
						Y: ptr(float64(y)),
					},
					nil,
					obs.SetSceneItemPropertiesScale{
						X: ptr(1.0/8.0),
						Y: ptr(1.0/10.0),
					},
					obs.SetSceneItemPropertiesCrop{},
					ptr(true),
					ptr(true),
					obs.SetSceneItemPropertiesBounds{
						X: ptr(float64(cw / 8)),
						Y: ptr(float64(ch / 10)),
					},
				)
				y -= ch / 10
				if err != nil {
					fmt.Println("Failed to set scene item properties:", err)
					return
				}
			}
		}
	}
	// Create wall scene.
	fmt.Println("Populating wall scene.")
	if info.Wall {
		iw, ih := cw/info.WallX, ch/info.WallY
		for x := uint(0); x < info.WallX; x++ {
			for y := uint(0); y < info.WallY; y++ {
				num := info.WallX*y + x + 1
				source := fmt.Sprintf("MC %d", num)
				res, err := o.AddSceneItem(
					"Wall",
					source,
					ptr(true),
				)
				if err != nil {
					fmt.Println("Failed to add scene item:", err)
					return
				}
				_, err = o.SetSceneItemProperties(
					"Wall",
					obs.SetSceneItemPropertiesItem{
						Id: ptr(res.ItemId),
					},
					obs.SetSceneItemPropertiesPosition{
						X: ptr(float64(x * iw)),
						Y: ptr(float64(y * ih)),
					},
					nil,
					obs.SetSceneItemPropertiesScale{
						X: ptr(1.0/float64(info.WallX)),
						Y: ptr(1.0/float64(info.WallY)),
					},
					obs.SetSceneItemPropertiesCrop{},
					ptr(true),
					ptr(true),
					obs.SetSceneItemPropertiesBounds{
						X: ptr(float64(iw)),
						Y: ptr(float64(ih)),
					},
				)
				if err != nil {
					fmt.Println("Failed to set scene item properties:", err)
					return
				}
			}
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}
