package ui

import (
	"fmt"
	"resetti/mc"
	"runtime"
	"time"
)

var logCh = make(chan string, 32)
var stateCh = make(chan []mc.Instance, 32)

func Log(content ...any) {
	prefix := fmt.Sprintf(
		"[%s] [INFO] ",
		time.Now().Format("15:04:05"),
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		logCh <- line
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]) + "\n"
		logCh <- line
	}
}

func LogError(content ...any) {
	pc, _, linenr, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	prefix := fmt.Sprintf(
		"[%s] [ERROR] %s:%d | ",
		time.Now().Format("15:04:05"),
		fn.Name(),
		linenr,
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		logCh <- line
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]) + "\n"
		logCh <- line
	}
}

func UpdateInstance(i ...mc.Instance) {
	stateCh <- i
}
