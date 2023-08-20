package ctl

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/log"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/exp/slices"
)

// ProjectorController maintains information about the state of the OBS
// projector and the mouse pointer.
type ProjectorController struct {
	conf *cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	Active bool // Whether the projector is focused

	winWidth, winHeight   int             // Projector window size
	BaseWidth, BaseHeight int             // OBS canvas size
	display               cfg.Rectangle   // Visible area of the projector window
	scale                 float64         // The scale of the window size to display area.
	window                xproto.Window   // Projector window ID
	children              []xproto.Window // Projector window children IDs
	grab                  bool            // Whether the pointer is grabbed
}

// Focus finds the wall projector and focuses it.
func (p *ProjectorController) Focus() error {
	if err := p.findProjector(); err != nil {
		return fmt.Errorf("find projector: %w", err)
	}
	if err := p.x.FocusWindow(p.window); err != nil {
		return fmt.Errorf("focus projector: %w", err)
	}
	if p.conf.Delay.Warp > 0 {
		time.Sleep(time.Millisecond * time.Duration(p.conf.Delay.Warp))
		p.x.WarpPointer(p.winWidth/2, p.winHeight/2, p.window)
	}
	return nil
}

// ProcessEvent processes a single event.
func (p *ProjectorController) ProcessEvent(evt x11.Event) {
	switch evt := evt.(type) {
	case x11.FocusEvent:
		// HACK: We don't actually need the pointer grab here, but if we don't grab
		// it then OBS decides to for some reason. This prevents the game from being
		// able to grab the pointer quickly. There's no code in OBS for grabbing the
		// pointer, so it's likely somewhere in Qt and I'm not interested in digging
		// into more C(++) code at the moment.
		p.Active = slices.Contains(p.children, xproto.Window(evt))
		if p.grab && !p.Active {
			p.ungrabPointer()
		} else if !p.grab && p.Active {
			p.grabPointer()
		}
	case x11.ResizeEvent:
		if evt.Window != p.window {
			return
		}
		if err := p.updateProjector(p.window); err != nil {
			log.Error("ProjectorController: Failed to process resize event: %s", err)
		}
	}
}

// InBounds returns whether or not the given coordinates are outside the bounds
// of the projector window.
func (p *ProjectorController) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x <= p.winWidth && y <= p.winHeight
}

// Setup sets up the ProjectorController.
func (p *ProjectorController) Setup(conf *cfg.Profile, obs *obs.Client, x *x11.Client) error {
	p.conf = conf
	p.obs = obs
	p.x = x
	width, height, err := p.obs.GetCanvasSize()
	if err != nil {
		return fmt.Errorf("get video size: %w", err)
	}
	p.BaseWidth, p.BaseHeight = width, height
	if err := p.findProjector(); err != nil {
		return fmt.Errorf("find projector: %w", err)
	}
	return nil
}

// ToScreen translates the given OBS video coordinates to screen coordinates.
func (p *ProjectorController) ToScreen(x, y int) (int, int) {
	x = int(float64(x+int(p.display.X)) * p.scale)
	y = int(float64(y+int(p.display.Y)) * p.scale)
	return x, y
}

// ToVideo translates the given screen coordinates to OBS video coordinates.
func (p *ProjectorController) ToVideo(x, y int) (int, int) {
	x = int(float64(x-int(p.display.X)) / p.scale)
	y = int(float64(y-int(p.display.Y)) / p.scale)
	return x, y
}

// Unfocus signals that the user left the projector.
func (p *ProjectorController) Unfocus() {
	if p.grab {
		p.ungrabPointer()
	}
}

// findProjector finds the wall projector.
func (p *ProjectorController) findProjector() error {
	windows := p.x.GetWindowList()
	for _, win := range windows {
		title, err := p.x.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, p.conf.Wall.WallWindow) {
			return p.updateProjector(win)
		}
	}
	return errors.New("no projector found")
}

// grabPointer grabs the mouse pointer if it is currently not grabbed.
func (p *ProjectorController) grabPointer() {
	// OBS can still be grabbing the pointer, so retry with backoff.
	timeout := time.Millisecond
	for tries := 1; tries <= 5; tries += 1 {
		if err := p.x.GrabPointer(p.window, p.conf.Wall.ConfinePointer); err != nil {
			log.Error("Pointer grab failed: (%d/5): %s", tries, err)
		} else {
			log.Info("Grabbed pointer.")
			p.grab = true
			return
		}
		time.Sleep(timeout)
		timeout *= 4
	}
	log.Error("Pointer grab failed.")
}

// ungrabPointer ungrabs the mouse pointer if it is currently grabbed.
func (p *ProjectorController) ungrabPointer() {
	if err := p.x.UngrabPointer(); err != nil {
		log.Error("Failed to ungrab pointer: %s", err)
	} else {
		log.Info("Ungrabbed pointer.")
		p.grab = false
	}
}

// updateProjector updates the projector size.
func (p *ProjectorController) updateProjector(win xproto.Window) error {
	p.window = win
	width, height, err := p.x.GetWindowSize(win)
	if err != nil {
		return fmt.Errorf("get projector size: %w", err)
	}
	p.children = p.x.GetWindowChildren(win)

	// Calculate projector letterboxing. Reference:
	// https://github.com/obsproject/obs-studio/blob/1b708b312e00595277dbf871f8488820cba4540a/UI/display-helpers.hpp#L23
	// https://github.com/obsproject/obs-studio/blob/1b708b312e00595277dbf871f8488820cba4540a/UI/window-projector.cpp#L180
	p.winWidth, p.winHeight = int(width), int(height)
	baseRatio := float64(p.BaseWidth) / float64(p.BaseHeight)
	projRatio := float64(p.winWidth) / float64(p.winHeight)
	if projRatio > baseRatio {
		p.scale = float64(p.winHeight) / float64(p.BaseHeight)
	} else {
		p.scale = float64(p.winWidth) / float64(p.BaseWidth)
	}
	p.display.W = uint32(p.scale * float64(p.BaseWidth))
	p.display.H = uint32(p.scale * float64(p.BaseHeight))
	p.display.X = uint32(p.winWidth/2) - (p.display.W / 2)
	p.display.Y = uint32(p.winHeight/2) - (p.display.H / 2)
	return nil
}
