// Package srv implements a few miscellaneous services for resetti:
// - World deletion/moving
// - Reset counting
package srv

import (
	"os"
	"strconv"
	"strings"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/ui"
)

var conf cfg.Config
var resetHandle *os.File
var resetCount int

func Init() error {
	conf = cfg.GetConfig()
	if conf.General.CountResets {
		file, err := os.OpenFile(conf.General.CountPath, os.O_RDWR, 0644)
		if err != nil {
			return err
		}
		resetHandle = file
		buf := make([]byte, 16)
		n, err := resetHandle.Read(buf)
		if err != nil {
			return err
		}
		c, err := strconv.Atoi(strings.Trim(string(buf[:n]), "\n"))
		if err != nil {
			return err
		}
		resetCount = c
	}
	return nil
}

func Fini() {
	if resetHandle != nil {
		resetHandle.Close()
	}
}

func UpdateInstance(i ...mc.Instance) {
	ui.UpdateInstance(i...)
	if resetHandle != nil {
		resetUpdated := false
		for _, v := range i {
			if v.State.Identifier == mc.StateGenerating {
				resetCount += 1
				resetUpdated = true
			}
		}
		if resetUpdated {
			_, err := resetHandle.Write([]byte(strconv.Itoa(resetCount)))
			if err != nil {
				logger.LogError("Failed to update reset count: %s", err)
			}
			ui.UpdateResets(resetCount)
		}
	}
}
