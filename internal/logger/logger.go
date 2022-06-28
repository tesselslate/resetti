// Package logger implements a basic logging service.
package logger

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"
)

var sub chan<- string
var writer io.Writer

func SetWriter(w io.Writer) {
	writer = w
}

func Subscribe(ch chan<- string) {
	sub = ch
}

func Log(content ...any) {
	prefix := fmt.Sprintf(
		"[%s] [INFO] ",
		time.Now().Format("2006-01-02_15:04:05"),
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		write(line)
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]...) + "\n"
		write(line)
	}
}

func LogError(content ...any) {
	pc, _, linenr, _ := runtime.Caller(1)
	name := runtime.FuncForPC(pc).Name()
	name = strings.TrimPrefix(name, "github.com/woofdoggo/resetti/internal/")
	prefix := fmt.Sprintf(
		"[%s] [ERROR] %s:%d | ",
		time.Now().Format("2006-01-02_15:04:05"),
		name,
		linenr,
	)
	if len(content) == 1 {
		line := prefix + fmt.Sprint(content[0]) + "\n"
		write(line)
	} else {
		line := prefix + fmt.Sprintf(content[0].(string), content[1:]...) + "\n"
		write(line)
	}
}

func write(line string) {
	sub <- line
	_, err := writer.Write([]byte(line))
	if err != nil {
		sub <- fmt.Sprintf("failed to write log: %s", err)
	}
}
