package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const gvisorProviderName = "gvisor"

var languageImages = map[string]string{
	"python":     "python:3.12-slim",
	"javascript": "node:22-alpine",
	"bash":       "alpine:3.19",
	"go":         "golang:1.23-alpine",
	"ruby":       "ruby:3.3-alpine",
}

const defaultImage = "alpine:3.19"
const defaultMemoryMB = 256
const defaultCPUs = 0.5

type GVisorProvider struct{}

func (p *GVisorProvider) Name() string {
	return gvisorProviderName
}

func (p *GVisorProvider) Available() bool {
	cmd := exec.Command("docker", "info", "--format", "{{.DefaultRuntime}}")
	out, err := cmd.Output()
	if err != nil {
		cmd2 := exec.Command("docker", "info")
		if err2 := cmd2.Run(); err2 != nil {
			return false
		}
		return true
	}

	runtime := strings.TrimSpace(string(out))
	return runtime == "runsc" || p.hasRunscRuntime()
}

func (p *GVisorProvider) hasRunscRuntime() bool {
	cmd := exec.Command("docker", "info", "--format", "{{json .Runtimes}}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "runsc")
}

func (p *GVisorProvider) Run(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
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
		"--runtime=runsc",
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
			return nil, fmt.Errorf("docker execution failed: %w", err)
		}
	}

	output := parseOutput(stdout.Bytes())
	logs := splitLines(stderr.String())

	return &SandboxResult{
		Output:   output,
		Logs:     logs,
		ExitCode: exitCode,
		Duration: time.Since(start),
		Provider: gvisorProviderName,
	}, nil
}

func resolveImage(req SandboxRequest) string {
	if req.Image != "" {
		return req.Image
	}
	if img, ok := languageImages[strings.ToLower(req.Language)]; ok {
		return img
	}
	return defaultImage
}

func buildCommand(language, code string) (string, []string) {
	switch strings.ToLower(language) {
	case "python":
		return "", []string{"python3", "-c", code}
	case "javascript":
		return "", []string{"node", "-e", code}
	case "bash":
		return "", []string{"sh", "-c", code}
	case "go":
		return "sh", []string{"-c", fmt.Sprintf(`echo '%s' > /tmp/main.go && go run /tmp/main.go`, strings.ReplaceAll(code, "'", "'\"'\"'"))}
	default:
		return "", []string{"sh", "-c", code}
	}
}

func parseOutput(raw []byte) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]any{}
	}

	var parsed map[string]any
	if err := json.Unmarshal(trimmed, &parsed); err == nil {
		return parsed
	}

	return map[string]any{"raw": string(trimmed)}
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
