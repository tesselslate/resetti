package reset

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/woofdoggo/resetti/internal/ui"
	"golang.org/x/sys/unix"
)

type affinityUpdate struct {
	Id   int
	Cpus unix.CPUSet
}

type resetDisplay struct {
	instances  []Instance
	states     []InstanceState
	cancel     context.CancelFunc
	log        logger
	origWriter io.Writer

	keys     <-chan ui.Key
	updates  <-chan LogUpdate
	affinity <-chan affinityUpdate
	resets   <-chan int
	stop     chan<- struct{}
}

type logger struct {
	file     *os.File
	messages chan string
}

func (m *logger) Write(p []byte) (n int, err error) {
	m.messages <- string(p)
	n, err = m.file.Write(p)
	if err != nil {
		return
	}
	if n != len(p) {
		err = io.ErrShortWrite
		return
	}
	return
}

// newResetDisplay creates a new ResetDisplay.
func newResetDisplay(instances []Instance) resetDisplay {
	rInstances := make([]Instance, len(instances))
	copy(rInstances, instances)
	return resetDisplay{
		instances: rInstances,
		states:    make([]InstanceState, len(instances)),
	}
}

// Init initializes the state of the terminal and sets the log output.
func (d *resetDisplay) Init() (chan<- LogUpdate, chan<- affinityUpdate, chan<- int, <-chan struct{}, error) {
	// Setup terminal.
	err := ui.InitTerminal()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	keys := ui.Listen(ctx)
	d.cancel = cancel
	d.keys = keys

	// Setup log display.
	logPath, err := os.UserCacheDir()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	logFile, err := os.OpenFile(logPath+"/resetti.log", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	d.log = logger{
		file:     logFile,
		messages: make(chan string, 128),
	}
	d.origWriter = log.Writer()
	log.SetOutput(&d.log)

	// Setup communication channels.
	state := make(chan LogUpdate, 128)
	affinity := make(chan affinityUpdate, 128)
	stop := make(chan struct{})
	resets := make(chan int, 128)
	d.updates = state
	d.affinity = affinity
	d.stop = stop
	d.resets = resets
	return state, affinity, resets, stop, nil
}

// Run runs the reset display.
func (d *resetDisplay) Run(ctx context.Context, affinity bool) {
	go d.run(ctx, affinity)
}

func (d *resetDisplay) run(ctx context.Context, affinity bool) {
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGWINCH)
	defer signal.Stop(sigs)
	width, height, _ := ui.GetSize()
	affinities := make([]unix.CPUSet, len(d.instances))
	logMsgs := make([]string, 0, 64)
	numCpu := runtime.NumCPU()
	resets := 0
	for {
		// Process events.
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		case <-sigs:
			width, height, _ = ui.GetSize()
		case key := <-d.keys:
			if key == ui.KeyCtrlC {
				d.stop <- struct{}{}
				return
			}
		case update := <-d.updates:
			d.states[update.Id] = update.State
		case aff := <-d.affinity:
			affinities[aff.Id] = aff.Cpus
		case msg := <-d.log.messages:
			if len(logMsgs) < 64 {
				logMsgs = append(logMsgs, msg)
			} else {
				logMsgs = logMsgs[1:]
				logMsgs = append(logMsgs, msg)
			}
		case reset := <-d.resets:
			resets = reset
		}

		// Style definitions.
		cyan := ui.NewStyle().Foreground(ui.Cyan).Bold()
		plain := ui.NewStyle().Foreground(ui.White)

		// Draw UI.
		ui.ClearTerminal()
		neededWidth := 50
		if affinity {
			neededWidth += numCpu
		}
		if neededWidth > width || len(d.instances)+5 > height {
			ui.NewStyle().RenderAt("Terminal too small", 0, 0)
			continue
		}
		if affinity {
			cyan.RenderAt("ID Version State            Affinity", 3, 2)
		} else {
			cyan.RenderAt("ID Version State", 3, 2)
		}
		for id, inst := range d.instances {
			b := strings.Builder{}
			b.WriteString(rightPad(fmt.Sprintf("%d", id), 3))
			b.WriteString(rightPad(fmt.Sprintf("1.%d", inst.Version), 8))
			b.WriteString(rightPad(d.states[id].String(), 18))
			plain.RenderAt(b.String(), 3, id+3)
			cpuString(affinities[id], id)
		}
		details := []string{
			"Instances",
			strconv.Itoa(len(d.instances)),
			"Routines",
			strconv.Itoa(runtime.NumGoroutine()),
			"Resets",
			strconv.Itoa(resets),
		}
		start := 40
		if affinity {
			start += 2 + numCpu
		}
		cyan.RenderAt("Details", start, 2)
		for i := 0; i < len(details)/2; i++ {
			cyan.RenderAt(details[i*2]+": ", start, i+3)
			plain.RenderAt(details[i*2+1], start+2+len(details[i*2]), i+3)
		}
		cyan.RenderAt("Log:", 3, len(d.instances)+6)
		if len(logMsgs) == 0 {
			continue
		}
		msgCount := height - (len(d.instances) + 8)
		for i := 0; i < msgCount; i++ {
			if i > len(logMsgs)-1 {
				break
			}
			plain.RenderAt(logMsgs[i], 3, len(d.instances)+7+i)
		}
	}
}

// Fini cleans up the changes made by ResetDisplay.
func (d *resetDisplay) Fini() {
	d.cancel()
	d.log.file.Close()
	ui.FiniTerminal()
}

func cpuString(cpus unix.CPUSet, id int) {
	numCpu := runtime.NumCPU()
	for i := 0; i < numCpu; i++ {
		if cpus.IsSet(i) {
			ui.NewStyle().Background(ui.BrightGreen).RenderAt(" ", 31+i, id+3)
		}
	}
}

func rightPad(text string, strlen int) string {
	if len(text) >= strlen {
		return text
	}
	return text + strings.Repeat(" ", strlen-len(text))
}
