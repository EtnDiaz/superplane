package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const dockerProviderName = "docker"

type DockerProvider struct{}

func (p *DockerProvider) Name() string {
	return dockerProviderName
}

func (p *DockerProvider) Available() bool {
	return exec.Command("docker", "info").Run() == nil
}

func (p *DockerProvider) Run(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	start := time.Now()

	image := resolveImage(req)
	inputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	memMB := req.MemoryMB
	if memMB <= 0 {
		memMB = defaultMemoryMB
	}

	cpus := req.CPUs
	if cpus <= 0 {
		cpus = defaultCPUs
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	args := []string{
		"run", "--rm",
		fmt.Sprintf("--memory=%dm", memMB),
		fmt.Sprintf("--cpus=%.2f", cpus),
		"--network=none",
		"-e", fmt.Sprintf("SUPERPLANE_INPUT=%s", inputJSON),
	}

	entrypoint, cmdArgs := buildCommand(req.Language, req.Code)
	if entrypoint != "" {
		args = append(args, "--entrypoint", entrypoint)
	}

	args = append(args, image)
	args = append(args, cmdArgs...)

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("execution timed out after %s", timeout)
		} else {
			return nil, fmt.Errorf("docker run failed: %w", err)
		}
	}

	return &SandboxResult{
		Output:   parseOutput(stdout.Bytes()),
		Logs:     splitLines(stderr.String()),
		ExitCode: exitCode,
		Duration: time.Since(start),
		Provider: dockerProviderName,
	}, nil
}
