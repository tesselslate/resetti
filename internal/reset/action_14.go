package reset

import (
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/x11"
)

func v14_pause(x *x11.Client, inst Instance, time *xproto.Timestamp) {
	x.SendKeyDown(x11.KeyF3, inst.Wid, time)
	x.SendKeyPress(x11.KeyEscape, inst.Wid, time)
	x.SendKeyUp(x11.KeyF3, inst.Wid, time)

}

func v14_reset(x *x11.Client, inst Instance, time *xproto.Timestamp) {
	x.SendKeyPress(x11.KeyF12, inst.Wid, time)
}
