package godevwatch

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// KillProcessOnPort kills any process listening on the specified port
func KillProcessOnPort(port int) error {
	// Find PIDs listening on the port
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-sTCP:LISTEN", "-t")
	output, err := cmd.Output()
	if err != nil {
		// No process found on this port, which is fine
		return nil
	}

	pids := strings.TrimSpace(string(output))
	if pids == "" {
		return nil
	}

	log.Printf("\033[33mKilling process(es) on port %d (PID: %s)...\033[0m\n", port, pids)

	// Kill each PID
	for _, pid := range strings.Split(pids, "\n") {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}

		killCmd := exec.Command("kill", "-TERM", pid)
		if err := killCmd.Run(); err != nil {
			log.Printf("Warning: Failed to kill process %s: %v", pid, err)
		}
	}

	return nil
}
