package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// findStroppy locates the stroppy binary.
// Search order: STROPPY_BIN env → ./build/stroppy → $PATH
func findStroppy() (string, error) {
	if bin := os.Getenv("STROPPY_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}
	if _, err := os.Stat("./build/stroppy"); err == nil {
		return "./build/stroppy", nil
	}
	if path, err := exec.LookPath("stroppy"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("stroppy binary not found: set STROPPY_BIN, place it in ./build/, or add to PATH")
}

// runStroppy executes the stroppy CLI with the given args and extra env vars.
// Returns combined stdout+stderr output.
func runStroppy(ctx context.Context, args []string, env map[string]string) (string, error) {
	bin, err := findStroppy()
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, bin, args...)

	// Inherit current env and merge extras
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// Merge output — stderr often has useful info from k6
	output := strings.TrimSpace(stdout.String())
	errOutput := strings.TrimSpace(stderr.String())
	if errOutput != "" {
		if output != "" {
			output += "\n" + errOutput
		} else {
			output = errOutput
		}
	}

	if err != nil {
		if output != "" {
			return "", fmt.Errorf("%w\n%s", err, output)
		}
		return "", err
	}

	return output, nil
}
