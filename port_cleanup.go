package godevwatch

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
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

		// Try graceful shutdown with SIGTERM first
		killCmd := exec.Command("kill", "-TERM", pid)
		if err := killCmd.Run(); err != nil {
			log.Printf("Warning: Failed to send TERM signal to process %s: %v", pid, err)
			continue
		}

		// Wait briefly for process to terminate
		time.Sleep(100 * time.Millisecond)

		// Check if process is still running
		checkCmd := exec.Command("kill", "-0", pid)
		if err := checkCmd.Run(); err != nil {
			// Process is gone (kill -0 failed), success!
			continue
		}

		// Process still running, force kill with SIGKILL
		log.Printf("\033[33mProcess %s did not respond to TERM, forcing kill...\033[0m\n", pid)
		forceKillCmd := exec.Command("kill", "-9", pid)
		if err := forceKillCmd.Run(); err != nil {
			log.Printf("Warning: Failed to force kill process %s: %v", pid, err)
		}

		// Wait a bit more for forced kill to take effect
		time.Sleep(50 * time.Millisecond)
	}

	return nil
}
