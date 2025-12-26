package tray

import (
	"os/exec"
	"runtime"

	"github.com/rs/zerolog/log"
)

// openLogsFolder opens the logs folder in file explorer
func openLogsFolder() {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "logs")
	case "darwin":
		cmd = exec.Command("open", "logs")
	default: // Linux and others
		cmd = exec.Command("xdg-open", "logs")
	}

	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("Failed to open logs folder")
	}
}
