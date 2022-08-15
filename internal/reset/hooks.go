package reset

import (
	"log"
	"os/exec"
	"strings"
)

// runCmd runs the given command.
func runCmd(str string) {
	splits := strings.Split(str, " ")
	var cmd *exec.Cmd
	if len(splits) == 1 {
		cmd = exec.Command(splits[0])
	} else {
		cmd = exec.Command(splits[0], splits[1:]...)
	}
	err := cmd.Run()
	if err != nil {
		log.Printf("runCmd err: %s\n", err)
		return
	}
}
