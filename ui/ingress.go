package ui

import (
	"fmt"
	"io"
	"resetti/mc"
	"runtime"
	"time"
)

var logCh = make(chan string, 32)
var stateCh = make(chan []mc.Instance, 32)
var logWriter io.Writer

func SetLogWriter(w io.Writer) {
	logWriter = w
}

func Log(content ...any) {
	prefix := fmt.Sprintf(
		"[%s] [INFO] ",
		time.Now().Format("2006-01-02_15:04:05"),
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		logCh <- line
		logWriter.Write([]byte(line))
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]...) + "\n"
		logCh <- line
		logWriter.Write([]byte(line))
	}
}

func LogError(content ...any) {
	pc, _, linenr, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	prefix := fmt.Sprintf(
		"[%s] [ERROR] %s:%d | ",
		time.Now().Format("2006-01-02_15:04:05"),
		fn.Name(),
		linenr,
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		logCh <- line
		logWriter.Write([]byte(line))
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]...) + "\n"
		logCh <- line
		logWriter.Write([]byte(line))
	}
}

func UpdateInstance(i ...mc.Instance) {
	stateCh <- i
}
