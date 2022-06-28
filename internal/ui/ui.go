// Package ui implements the user interface of resetti.
package ui

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/terminal"
)

const (
	ch_size  = 32
	log_size = 128
)

var ErrShutdown = errors.New("shutting down")

var sub chan<- error
var msg chan string
var keys chan terminal.Key
var resetCh chan int
var stateCh chan []mc.Instance
var stop chan struct{}
var winCh chan os.Signal
var instances []mc.Instance
var startTime time.Time
var recentLog []string
var resets int
var logIdx = 0
var width int

func Subscribe(ch chan<- error) {
	sub = ch
}

func Init(i []mc.Instance) error {
	w, _, err := terminal.GetSize()
	if err != nil {
		return err
	}
	width = w
	msg = make(chan string, ch_size)
	keys = make(chan terminal.Key, ch_size)
	resetCh = make(chan int, ch_size)
	stateCh = make(chan []mc.Instance, ch_size)
	stop = make(chan struct{})
	winCh = make(chan os.Signal, ch_size)
	instances = make([]mc.Instance, len(i))
	copy(instances, i)
	startTime = time.Now()
	recentLog = make([]string, log_size)
	logger.Subscribe(msg)
	err = terminal.Init(keys)
	if err != nil {
		return err
	}
	signal.Notify(winCh, syscall.SIGWINCH)
	go run()
	return nil
}

func Fini() {
	stop <- struct{}{}
	<-stop
	terminal.Fini()
}

func UpdateInstance(i ...mc.Instance) {
	stateCh <- i
}

func UpdateResets(count int) {
	resetCh <- count
}

func run() {
	defer terminal.Fini()
	for {
		select {
		case logMsg := <-msg:
			recentLog[logIdx] = logMsg
			logIdx = (logIdx + 1) % log_size
		case key := <-keys:
			switch key {
			case terminal.KeyCtrlC:
				logger.Log("User pressed Ctrl+C, shutting down.")
				sub <- ErrShutdown
				return
			}
		case count := <-resetCh:
			resets = count
		case states := <-stateCh:
			if len(states) > len(instances) {
				instances = make([]mc.Instance, len(states))
				copy(instances, states)
			} else {
				for _, v := range states {
					instances[v.Id] = v
				}
			}
		case <-winCh:
			w, _, err := terminal.GetSize()
			if err != nil {
				logger.LogError("Failed to get terminal size: %s", err)
			}
			width = w
		case <-time.After(1 * time.Second):
			// Force UI updates to occur at least once per second.
		case <-stop:
			logger.Log("UI received shutdown notification.")
			stop <- struct{}{}
			return
		}
		// Render UI.
		terminal.Clear()
		cyan := terminal.NewStyle().Foreground(terminal.Cyan).Bold()
		cyan.RenderAt("ID  Version State", 3, 2)
		style := terminal.NewStyle().Foreground(terminal.White)
		for idx, i := range instances {
			str := pad(fmt.Sprintf("%d", i.Id), 4)
			str += pad(i.Version.String(), 8)
			str += pad(i.State.String(), 16)
			style.RenderAt(str, 3, idx+3)
		}
		boldStyle := terminal.NewStyle().Foreground(terminal.Cyan).Bold()
		regStyle := terminal.NewStyle()
		details := []string{
			"Instances",
			strconv.Itoa(len(instances)),
			"Routines",
			strconv.Itoa(runtime.NumGoroutine()),
			"Uptime",
			prettifyTime(time.Since(startTime)),
			"Resets",
			strconv.Itoa(resets),
		}
		cyan.RenderAt("Details", 40, 2)
		for i := 0; i < len(details)/2; i += 1 {
			boldStyle.RenderAt(details[i*2]+": ", 40, i+3)
			regStyle.RenderAt(details[i*2+1], 40+len(details[i*2])+2, i+3)
		}
		terminal.NewStyle().Foreground(terminal.Gray).RenderAt("ctrl+c: exit", 3, len(instances)+4)
		cyan.RenderAt("Log:", 3, len(instances)+6)
		inc := 0
		for i := 10; i > 0; i -= 1 {
			msg := recentLog[(logIdx+100-i)%100]
			if msg == "" {
				continue
			}
			if len(msg) > width-3 {
				msg = msg[:width-3]
			}
			terminal.NewStyle().RenderAt(msg, 3, len(instances)+6+inc)
			inc += 1
		}
	}
}

func pad(txt string, strlen int) string {
	if len(txt) >= strlen {
		return txt
	}
	return txt + strings.Repeat(" ", strlen-len(txt))
}

func prettifyTime(t time.Duration) string {
	if math.Floor(t.Hours()) > 0 {
		return fmt.Sprintf(
			"%02d:%02d:%02d",
			int(math.Floor(t.Hours()))%60,
			int(math.Floor(t.Minutes()))%60,
			int(math.Floor(t.Seconds()))%60,
		)
	} else if math.Floor(t.Minutes()) > 0 {
		return fmt.Sprintf(
			"%02d:%02d",
			int(math.Floor(t.Minutes()))%60,
			int(math.Floor(t.Seconds()))%60,
		)
	}
	return fmt.Sprintf("%.0f sec", math.Floor(t.Seconds()))
}
