package manager

import (
	"os/exec"
	"strings"

	"github.com/woofdoggo/resetti/internal/logger"
)

func runHook(str string) {
	splits := strings.Split(str, " ")
	var cmd *exec.Cmd
	if len(splits) == 1 {
		cmd = exec.Command(splits[0])
	} else {
		cmd = exec.Command(splits[0], splits[1:]...)
	}
	err := cmd.Run()
	if err != nil {
		logger.LogError("Failed to run hook: %s", err)
		return
	}
}
