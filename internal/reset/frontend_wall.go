package reset

import (
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/x11"
)

type FrontendWall struct {
}

func (f *FrontendWall) HandleInput(event x11.Event) error {
	panic("TODO")
}

func (f *FrontendWall) HandleUpdate(update mc.Update) error {
	panic("TODO")
}

func (f *FrontendWall) Setup(opts FrontendOptions) error {
	panic("TODO")
}

func (f *FrontendWall) ShouldPause(id int) bool {
	panic("TODO")
}
