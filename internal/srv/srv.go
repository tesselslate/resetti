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
var resetCh chan int
var stopCh chan struct{}

func Init() error {
	resetCh = make(chan int, 32)
	stopCh = make(chan struct{})
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
	go updateResets()
	return nil
}

func Fini() {
	stopCh <- struct{}{}
	if resetHandle != nil {
		resetHandle.Close()
	}
}

func updateResets() {
	for {
		select {
		case resets := <-resetCh:
			resetCount += resets
			_, err := resetHandle.Seek(0, 0)
			if err != nil {
				logger.LogError("Failed to update reset count: %s", err)
				return
			}
			_, err = resetHandle.WriteString(strconv.Itoa(resetCount))
			if err != nil {
				logger.LogError("Failed to update reset count: %s", err)
			}
			ui.UpdateResets(resetCount)
		case <-stopCh:
			return
		}
	}
}

func UpdateInstance(i ...mc.Instance) {
	ui.UpdateInstance(i...)
	if resetHandle != nil {
		resetUpdated := false
		toAdd := 0
		for _, v := range i {
			if v.State.Identifier == mc.StateGenerating {
				toAdd += 1
				resetUpdated = true
			}
		}
		if resetUpdated {
			resetCh <- toAdd
		}
	}
}
