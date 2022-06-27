// Package terminal implements basic operations for working with the terminal.
package terminal

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/term"
)

var initialTermState *term.State
var newSub chan chan<- Key

func init() {
	newSub = make(chan chan<- Key, 1)
	go listen()
}

func Init(ch chan<- Key) error {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	initialTermState = state
	fmt.Print("\x1b[?25l\x1b[?1049h")
	newSub <- ch
	return nil
}

func Fini() {
	fmt.Print("\x1b[?25h\x1b[?1049l")
	_ = term.Restore(int(os.Stdin.Fd()), initialTermState)
}

func GetSize() (int, int, error) {
	return term.GetSize(int(os.Stdin.Fd()))
}

func Clear() {
	fmt.Print("\x1b[2J")
}

func listen() {
	var sub chan<- Key = nil
	reader := bufio.NewReader(os.Stdin)
	buf := make([]byte, 0)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		select {
		case newCh := <-newSub:
			sub = newCh
		default:
		}
		if sub == nil {
			continue
		}
		if len(buf) == 0 {
			if b != 0x1b {
				switch b {
				case 3:
					sub <- KeyCtrlC
				case 18:
					sub <- KeyCtrlR
				default:
					sub <- Key(b)
				}
				continue
			}
			buf = []byte{b}
			continue
		}
		buf = append(buf, b)
		if len(buf) > 1 {
			if len(buf) > 3 {
				// Unknown key
				buf = make([]byte, 0)
				continue
			}
			if buf[1] != '[' {
				sub <- '\x1b'
				switch buf[1] {
				case 3:
					sub <- KeyCtrlC
				case 18:
					sub <- KeyCtrlR
				default:
					sub <- Key(buf[1])
				}
				buf = make([]byte, 0)
				continue
			}
			switch string(buf) {
			case "\x1b[A":
				sub <- KeyUp
			case "\x1b[B":
				sub <- KeyDown
			case "\x1b[D":
				sub <- KeyLeft
			case "\x1b[C":
				sub <- KeyRight
			default:
				continue
			}
			buf = make([]byte, 0)
		} else {
			continue
		}
	}
}
