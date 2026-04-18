package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// dohup is a Linux-only utility to ensure a subprocess gets properly terminated
// when dohup receives termination signals. It implements graceful shutdown with
// a timeout before sending SIGKILL.

const (
	// gracefulTimeout is the time to wait for graceful shutdown before SIGKILL
	gracefulTimeout = 10 * time.Second
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: dohup <command> [args...]")
		os.Exit(1)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting command: %v\n", err)
		os.Exit(1)
	}

	// Channel to signal when the process exits
	done := make(chan error, 1)

	// Wait for the command to finish in a separate goroutine
	go func() {
		done <- cmd.Wait()
	}()

	// Set up signal handling for Linux signals
	sigChan := make(chan os.Signal, 1)
	// Only listen for signals we care about
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case err := <-done:
		// Process finished normally, propagate exit code
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			} else {
				fmt.Fprintf(os.Stderr, "Error waiting for command: %v\n", err)
				os.Exit(1)
			}
		}
		os.Exit(0)

	case sig := <-sigChan:
		if sig == syscall.SIGHUP {
			sig = syscall.SIGTERM // Treat SIGHUP as SIGTERM for the subprocess
		}

		// For SIGHUP, we still want to terminate the subprocess
		// Start graceful shutdown process
		if err := terminateProcess(cmd, sig, done); err != nil {
			fmt.Fprintf(os.Stderr, "Error terminating subprocess: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

// terminateProcess implements graceful shutdown with timeout
func terminateProcess(cmd *exec.Cmd, sig os.Signal, done chan error) error {
	if cmd.Process == nil {
		return fmt.Errorf("no process to terminate")
	}

	// First, send the specified signal for graceful shutdown
	if err := cmd.Process.Signal(sig); err != nil {
		// If sending the signal fails, try SIGKILL immediately
		return cmd.Process.Kill()
	}

	// Wait for graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), gracefulTimeout)
	defer cancel()

	select {
	case <-done:
		// Process exited gracefully
		return nil
	case <-ctx.Done():
		// Timeout reached, force kill
		fmt.Fprintf(os.Stderr, "Graceful shutdown timeout, sending SIGKILL...\n")
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		// Wait a bit for the kill to take effect
		select {
		case <-done:
			return nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("process did not respond to SIGKILL")
		}
	}
}
