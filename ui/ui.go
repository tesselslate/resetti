// Package ui implements the user interface of resetti.
package ui

import (
	"fmt"
	"github.com/woofdoggo/resetti/mc"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const ALTER_ENTER = "\x1b[?25l\x1b[?1049h"
const ALTER_EXIT = "\x1b[?25h\x1b[?1049l"
const CLEAR_END = "\x1b[J"
const CURSOR_START = "\x1b[H"
const DETAILS_BOLD = "\x1b[0;36m"
const DETAILS_REG = "\x1b[0;37m"
const INSTANCES_BOLD = "\x1b[1;36m"
const INSTANCES_REG = "\x1b[0;37m"
const RESET_STYLE = "\x1b[0m"
const TIPS_COLOR = "\x1b[0;38;5;248m"

type Ui struct {
	Errors      chan error
	stop        chan struct{}
	instances   []mc.Instance
	recentLog   []string
	start       time.Time
	logCount    int
	resetCount  int
	resetHandle *os.File
}

func (u *Ui) Start(instances []mc.Instance, countHandle *os.File) {
	u.Errors = make(chan error, 4)
	u.stop = make(chan struct{})
	u.instances = make([]mc.Instance, len(instances))
	copy(u.instances, instances)
	u.recentLog = make([]string, 0)
	u.start = time.Now()
	u.resetHandle = countHandle
	fmt.Print(ALTER_ENTER)
	go u.run()
}

func (u *Ui) Stop() {
	u.stop <- struct{}{}
}

func (u *Ui) run() {
	bytebuf := make([]byte, 16)
	n, err := u.resetHandle.Read(bytebuf)
	if err != nil {
		fmt.Print(ALTER_EXIT)
		u.Errors <- err
		return
	}
	count, err := strconv.Atoi(strings.Trim(string(bytebuf[:n]), "\n"))
	if err != nil {
		fmt.Print(ALTER_EXIT)
		u.Errors <- err
		return
	}
	u.resetCount = count
	for {
		// Process incoming UI updates.
		select {
		case logMsg := <-logCh:
			if len(u.recentLog) > 10 {
				u.recentLog = append(u.recentLog[1:], logMsg)
			} else {
				u.recentLog = append(u.recentLog, logMsg)
			}
			u.logCount += 1
		case instances := <-stateCh:
			if len(instances) == 1 {
				id := instances[0].Id
				u.instances[id] = instances[0]
			} else {
				u.instances = make([]mc.Instance, len(instances))
				copy(u.instances, instances)
			}
			for _, v := range instances {
				if v.State == mc.StateGenerating {
					u.resetCount += 1
					_, err := u.resetHandle.Seek(0, 0)
					if err != nil {
						LogError("Failed to seek in reset file: %s", err)
						continue
					}
					_, err = u.resetHandle.WriteString(strconv.Itoa(u.resetCount))
					if err != nil {
						LogError("Failed to write reset count of %d: %s", u.resetCount, err)
					}
				}
			}
		case <-time.After(1 * time.Second):
			// Timeout to force UI updates at least once per second.
		case <-u.stop:
			fmt.Print(ALTER_EXIT)
			Log("UI received shutdown notification.")
			return
		}
		// Render new UI.
		fmt.Print(CURSOR_START, CLEAR_END, "\n")
		instances := make([]string, 0, len(u.instances))
		for _, i := range u.instances {
			str := INSTANCES_REG + "  "
			str += pad(fmt.Sprintf("%d", i.Id), 4)
			str += pad(i.Version.String(), 8)
			str += pad(i.State.String(), 16)
			instances = append(instances, str)
		}
		uptime := time.Since(u.start)
		details := []string{
			fmt.Sprintf("%sInstances: %s%d", DETAILS_BOLD, DETAILS_REG, len(instances)),
			fmt.Sprintf("%sResets: %s%d", DETAILS_BOLD, DETAILS_REG, u.resetCount),
			fmt.Sprintf("%sRoutines: %s%d", DETAILS_BOLD, DETAILS_REG, runtime.NumGoroutine()),
			fmt.Sprintf("%sLog Count: %s%d", DETAILS_BOLD, DETAILS_REG, u.logCount),
			fmt.Sprintf("%sUptime: %s%s", DETAILS_BOLD, DETAILS_REG, prettifyTime(uptime)),
		}
		fmt.Printf("%s  ID  Version State           Details\n", INSTANCES_BOLD)
		rows := max(len(details), len(instances))
		for i := 0; i < rows; i++ {
			if i < len(instances) {
				fmt.Print(instances[i])
			} else {
				fmt.Print(pad("", 31))
			}
			if i < len(details) {
				fmt.Print(details[i])
			}
			fmt.Print("\n")
		}
		fmt.Printf("\n%s  ctrl+c: exit%s\n\n", TIPS_COLOR, RESET_STYLE)
		fmt.Print("\n\n")
		for _, msg := range u.recentLog {
			fmt.Print(msg)
		}
	}
}

func pad(i string, strlen int) string {
	if strlen-len(i) <= 0 {
		return i
	}
	return i + strings.Repeat(" ", strlen-len(i))
}

func prettifyTime(t time.Duration) string {
	if math.Floor(t.Hours()) > 0 {
		return fmt.Sprintf(
			"%d:%d:%d",
			int(math.Floor(t.Hours()))%60,
			int(math.Floor(t.Minutes()))%60,
			int(math.Floor(t.Seconds()))%60,
		)
	} else if math.Floor(t.Minutes()) > 0 {
		return fmt.Sprintf(
			"%d:%d",
			int(math.Floor(t.Minutes()))%60,
			int(math.Floor(t.Seconds()))%60,
		)
	}
	return fmt.Sprintf("%.0f sec", math.Floor(t.Seconds()))
}

func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}
