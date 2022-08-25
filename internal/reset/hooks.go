package reset

import (
	"log"
	"os/exec"
	"strings"
)

// runHook runs the given command.
func runHook(str string) {
	if str == "" {
		return
	}
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
