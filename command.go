package godevwatch

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Command represents a shell command with process management
type Command struct {
	cmdString string
	cmd       *exec.Cmd
	OnStdout  func(string)
	OnStderr  func(string)
}

// NewCommand creates a new command
func NewCommand(cmdString string) *Command {
	return &Command{
		cmdString: cmdString,
	}
}

// Run executes the command and waits for it to complete
func (c *Command) Run() error {
	// Parse command string into shell execution
	c.cmd = exec.Command("sh", "-c", c.cmdString)
	c.cmd.Env = os.Environ()

	// Set process group so we can kill child processes
	c.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Setup stdout pipe
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Setup stderr pipe
	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output
	go c.streamOutput(stdout, c.OnStdout)
	go c.streamOutput(stderr, c.OnStderr)

	// Wait for command to complete
	return c.cmd.Wait()
}

// streamOutput streams output line by line
func (c *Command) streamOutput(reader io.Reader, callback func(string)) {
	if callback == nil {
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		callback(scanner.Text())
	}
}

// Kill terminates the command and all child processes
func (c *Command) Kill() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	// Kill the process group to ensure all children are terminated
	pgid, err := syscall.Getpgid(c.cmd.Process.Pid)
	if err != nil {
		// Fallback to killing just the process
		return c.cmd.Process.Kill()
	}

	// Send SIGTERM to the process group
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return err
	}

	return nil
}

// parseCommand parses a command string into program and arguments
func parseCommand(cmdString string) (string, []string) {
	parts := strings.Fields(cmdString)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
