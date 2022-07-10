package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	obs "github.com/woofdoggo/go-obs"
	"github.com/woofdoggo/resetti/internal/terminal"
)

type obsMenu struct {
	instances        int
	wallWidth        int
	wallHeight       int
	verificationPos  int
	verificationSize int
	verification     int
	lockImg          string
	selected         int
	width            int
	height           int
}

const (
	topLeft int = iota
	left
	bottomLeft
	topRight
	right
	bottomRight

	verifScene   = 1
	verifSources = 2
)

var (
	goldStyle       terminal.Style = terminal.NewStyle().Foreground(terminal.BrightYellow).Bold()
	grayStyle                      = terminal.NewStyle().Foreground(terminal.Gray)
	redStyle                       = terminal.NewStyle().Foreground(terminal.BrightRed).Bold()
	selectedStyle                  = terminal.NewStyle().Foreground(terminal.BrightMagenta).Bold()
	unselectedStyle                = terminal.NewStyle().Foreground(terminal.White)
	whiteStyle                     = unselectedStyle

	directions []rune  = []rune("↖←↙↗→↘")
	clamps     [][]int = [][]int{
		{1, 99, 1},
		{1, 99, 1},
		{1, 99, 1},
		{0, 5, 1},
		{1, 10, 1},
		{0, 2, 1},
	}
	methods []string = []string{
		"None",
		"Wall",
		"Inst",
	}
)

func CmdObs() {
	// Parse CLI arguments and connect to OBS.
	m := obsMenu{
		instances:        1,
		wallWidth:        1,
		wallHeight:       1,
		verificationSize: 6,
	}
	port := 4440
	pass := ""
	hadArgs := false
	for _, v := range os.Args {
		if strings.HasPrefix(v, "--port=") {
			if len(v) == 7 {
				fmt.Println("bad argument")
				os.Exit(1)
			}
			num, err := strconv.Atoi(v[7:])
			if err != nil {
				fmt.Println("bad argument:", err)
				os.Exit(1)
			}
			port = num
			hadArgs = true
		} else if strings.HasPrefix(v, "--pass=") {
			if len(v) == 7 {
				fmt.Println("bad argument")
				os.Exit(1)
			}
			pass = v[7:]
			hadArgs = true
		} else if strings.HasPrefix(v, "--lockImg=") {
			if len(v) == 10 {
				fmt.Println("bad argument")
				os.Exit(1)
			}
			m.lockImg = v[10:]
		}
	}
	if !hadArgs {
		fmt.Println("Please supply a port and password to connect to OBS.")
		fmt.Println("resetti obs [...]")
		fmt.Println("   --port=PORT         The port to connect to OBS on.")
		fmt.Println("   --pass=PASSWORD     The password to connect to OBS with.")
		fmt.Println("   --lockImg=PATH      Path to lock image (optional.)")
		os.Exit(1)
	}
	o := obs.Client{}
	needAuth, _, err := o.Connect(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		fmt.Println("Failed to connect to OBS:", err)
		os.Exit(1)
	}
	if needAuth {
		_, err := o.Authenticate(pass)
		if err != nil {
			fmt.Println("Failed to authenticate with OBS:", err)
			os.Exit(1)
		}
	}
	// Start menu.
	keys := make(chan terminal.Key, 32)
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGWINCH)
	w, h, err := terminal.GetSize()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	m.width = w
	m.height = h
	err = terminal.Init(keys)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	m.Draw()
	for {
		select {
		case k := <-keys:
			switch k {
			case terminal.KeyLeft:
				if m.selected == 6 {
					continue
				}
				value := m.getSelectedValue()
				if clamps[m.selected][0] < value {
					m.setSelectedValue(value - clamps[m.selected][2])
					m.Draw()
				}
			case terminal.KeyRight:
				if m.selected == 6 {
					continue
				}
				value := m.getSelectedValue()
				if clamps[m.selected][1] > value {
					m.setSelectedValue(value + clamps[m.selected][2])
					m.Draw()
				}
			case terminal.KeyUp:
				if m.selected != 0 {
					m.selected -= 1
					m.Draw()
				}
			case terminal.KeyDown, terminal.KeyEnter:
				if m.selected != 6 {
					m.selected += 1
					m.Draw()
				} else if m.instances <= m.wallWidth*m.wallHeight {
					if k == terminal.KeyEnter {
						terminal.Fini()
						generateScenes(m, &o)
						return
					}
				}
			case terminal.KeyCtrlC:
				terminal.Fini()
				return
			}
		case <-sigs:
			w, h, err = terminal.GetSize()
			if err != nil {
				m.width = w
				m.height = h
				m.Draw()
			}
		}
	}
}

func generateScenes(m obsMenu, o *obs.Client) {
	videoSettings, err := o.GetVideoInfo()
	if err != nil {
		fmt.Println("Failed to get resolution:", err)
		return
	}
	_, err = o.SetCurrentSceneCollection(fmt.Sprintf("resetti - %d multi", m.instances))
	if err != nil {
		fmt.Println("Failed to set scene collection:", err)
		return
	}
	cw, ch := videoSettings.BaseWidth, videoSettings.BaseHeight
	_, err = o.CreateScene("Wall")
	if err != nil {
		fmt.Println("Failed to create scene:", err)
		return
	}
	for i := 0; i < m.instances; i++ {
		scene := fmt.Sprintf("Instance %d", i+1)
		source := fmt.Sprintf("MC %d", i+1)
		_, err := o.CreateScene(scene)
		if err != nil {
			fmt.Println("Failed to create scene:", err)
			return
		}
		_, err = o.CreateSource(
			source,
			"xcomposite_input",
			"Scene",
			nil,
			ptr(true),
		)
		if err != nil {
			fmt.Println("Failed to create source:", err)
			return
		}
		_, err = o.AddFilterToSource(
			source,
			"Scaling/Aspect Ratio",
			"scale_filter",
			map[string]string{
				"resolution": fmt.Sprintf("%dx%d", cw, ch),
			},
		)
		if err != nil {
			fmt.Println("Failed to add scale filter to instance:", err)
			return
		}
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
			obs.SetSceneItemPropertiesScale{},
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
	}
	for i := 0; i < m.instances; i++ {
		if m.verification == 0 {
			continue
		}
		scene := fmt.Sprintf("Instance %d", i+1)
		var x, y, count int
		if m.verification == verifScene {
			count = 1
		} else {
			count = m.instances - 1
		}
		ws, hs := 16.0/float64(m.verificationSize), 36.0/float64(m.verificationSize)
		w, h := int(float64(cw)/ws), int(float64(ch)/hs)
		switch m.verificationPos {
		case topLeft:
			x, y = 0, 0
		case topRight:
			x, y = cw-w, 0
		case bottomLeft:
			x, y = 0, ch-(count*h)
		case bottomRight:
			x, y = cw-w, ch-(count*h)
		case left:
			x, y = 0, (ch/2 - (count*h)/2)
		case right:
			x, y = cw-w, (ch/2 - (count*h)/2)
		}
		if m.verification == verifScene {
			res, err := o.AddSceneItem(
				scene,
				"Wall",
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
					X: ptr(1.0 / ws),
					Y: ptr(1.0 / hs),
				},
				obs.SetSceneItemPropertiesCrop{},
				ptr(true),
				ptr(true),
				obs.SetSceneItemPropertiesBounds{
					X: ptr(float64(cw) / ws),
					Y: ptr(float64(ch) / hs),
				},
			)
			if err != nil {
				fmt.Println("Failed to set scene item properties:", err)
				return
			}
		} else {
			for j := 0; j < m.instances; j++ {
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
						X: ptr(1.0 / ws),
						Y: ptr(1.0 / hs),
					},
					obs.SetSceneItemPropertiesCrop{},
					ptr(true),
					ptr(true),
					obs.SetSceneItemPropertiesBounds{
						X: ptr(float64(w)),
						Y: ptr(float64(h)),
					},
				)
				if err != nil {
					fmt.Println("Failed to set scene item properties:", err)
					return
				}
				y += h
			}
		}
	}
	iw, ih := int(cw)/m.wallWidth, int(ch)/m.wallHeight
	for x := 0; x < m.wallWidth; x++ {
		for y := 0; y < m.wallHeight; y++ {
			num := m.wallWidth*y + x + 1
			if num > m.instances {
				break
			}
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
					X: ptr(1.0 / float64(m.wallWidth)),
					Y: ptr(1.0 / float64(m.wallHeight)),
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
			_, err = o.CreateSource(
				fmt.Sprintf("Lock %d", num),
				"image_source",
				"Wall",
				map[string]interface{}{
					"file": m.lockImg,
				},
				ptr(true),
			)
			if err != nil {
				fmt.Println("Failed to create lock image:", err)
				return
			}
			_, err = o.SetSceneItemProperties(
				"Wall",
				obs.SetSceneItemPropertiesItem{
					Name: fmt.Sprintf("Lock %d", num),
				},
				obs.SetSceneItemPropertiesPosition{
					X: ptr(float64(x * iw)),
					Y: ptr(float64(y * ih)),
				},
				nil,
				obs.SetSceneItemPropertiesScale{},
				obs.SetSceneItemPropertiesCrop{},
				ptr(true),
				ptr(true),
				obs.SetSceneItemPropertiesBounds{},
			)
			if err != nil {
				fmt.Println("Failed to set lock properties:", err)
				return
			}
		}
	}
	fmt.Println("Done! You can delete the scene named 'Scene' if you would like.")
}

func (m *obsMenu) getSelectedValue() int {
	switch m.selected {
	case 0:
		return m.instances
	case 1:
		return m.wallWidth
	case 2:
		return m.wallHeight
	case 3:
		return m.verificationPos
	case 4:
		return m.verificationSize
	case 5:
		return m.verification
	default:
		return 0
	}
}

func (m *obsMenu) setSelectedValue(val int) {
	switch m.selected {
	case 0:
		m.instances = val
	case 1:
		m.wallWidth = val
	case 2:
		m.wallHeight = val
	case 3:
		m.verificationPos = val
	case 4:
		m.verificationSize = val
	case 5:
		m.verification = val
	}
}

func (m *obsMenu) getStyle(row int) terminal.Style {
	if m.selected == row {
		return selectedStyle
	} else {
		return unselectedStyle
	}
}

func (m *obsMenu) selToRow() int {
	if m.selected < 6 {
		return m.selected + 1
	} else {
		return 8
	}
}

func (m *obsMenu) renderArrows(row int, num int) {
	value := m.getSelectedValue()
	numLen := 0
	if num < 10 {
		numLen = 1
	} else if num < 100 {
		numLen = 2
	} else {
		numLen = 3
	}
	x := m.width/2 + 3 - numLen
	y := m.height/2 - 4 + row
	whiteStyle.RenderAt(fmt.Sprintf("<%s>", strings.Repeat(" ", numLen+2)), x, y)
	switch value {
	case clamps[m.selected][0]:
		grayStyle.RenderAt("<", x, y)
	case clamps[m.selected][1]:
		grayStyle.RenderAt(">", x+numLen+3, y)
	}
	selectedStyle.RenderAt(strconv.Itoa(num), x+2, y)
}

func (m *obsMenu) renderRight(row int, str string) {
	x := m.width/2 + 5 - len(str)
	grayStyle.RenderAt(str, x, m.height/2-4+row)
}

func (m *obsMenu) Draw() {
	terminal.Clear()
	if m.width < 44 || m.height < 18 {
		terminal.NewStyle().RenderAt("Terminal too small", 0, 0)
		return
	}
	sx, sy := m.width/2-20, m.height/2-5
	terminal.NewStyle().Foreground(terminal.Cyan).RenderAt("use arrow keys to navigate", sx, sy+10)
	redStyle.RenderAt("make a scene collection named:", sx, sy+12)
	redStyle.RenderAt(fmt.Sprintf("resetti - %d multi", m.instances), sx, sy+13)
	redStyle.RenderAt("before pressing Finish", sx, sy+14)
	goldStyle.RenderAt("OBS Setup", sx, sy)
	// Cursor
	goldStyle.RenderAt(">", sx-2, sy+m.selToRow())
	// Instance count
	m.getStyle(0).RenderAt("Instances", sx, sy+1)
	if m.selected == 0 {
		m.renderArrows(0, m.instances)
	} else {
		m.renderRight(0, strconv.Itoa(m.instances))
	}
	// Wall rows
	m.getStyle(1).RenderAt("Wall Cols", sx, sy+2)
	if m.selected == 1 {
		m.renderArrows(1, m.wallWidth)
	} else {
		m.renderRight(1, strconv.Itoa(m.wallWidth))
	}
	// Wall columns
	m.getStyle(2).RenderAt("Wall Rows", sx, sy+3)
	if m.selected == 2 {
		m.renderArrows(2, m.wallHeight)
	} else {
		m.renderRight(2, strconv.Itoa(m.wallHeight))
	}
	// Verification position
	{
		m.getStyle(3).RenderAt("Verif. Position", sx, sy+4)
		var style terminal.Style
		if m.selected == 3 {
			style = selectedStyle
			whiteStyle.RenderAt("<   >", sx+22, sy+4)
			switch m.verificationPos {
			case clamps[3][0]:
				grayStyle.RenderAt("<", sx+22, sy+4)
			case clamps[3][1]:
				grayStyle.RenderAt(">", sx+26, sy+4)
			}
		} else {
			style = grayStyle
		}
		style.RenderAt(string(directions[m.verificationPos]), sx+24, sy+4)
	}
	// Verification size
	m.getStyle(4).RenderAt("Verif. Scale", sx, sy+5)
	if m.selected == 4 {
		m.renderArrows(4, m.verificationSize)
	} else {
		m.renderRight(4, strconv.Itoa(m.verificationSize))
	}
	// Verification method
	m.getStyle(5).RenderAt("Verif. Method", sx, sy+6)
	if m.selected == 5 {
		whiteStyle.RenderAt("<      >", sx+19, sy+6)
		switch m.verification {
		case clamps[5][0]:
			grayStyle.RenderAt("<", sx+19, sy+6)
		case clamps[5][1]:
			grayStyle.RenderAt(">", sx+26, sy+6)
		}
		selectedStyle.RenderAt(methods[m.verification], sx+21, sy+6)
	} else {
		m.renderRight(5, methods[m.verification])
	}
	// Done
	m.getStyle(6).RenderAt("Finish", sx, sy+8)
}

func ptr[T any](val T) *T {
	return &val
}
