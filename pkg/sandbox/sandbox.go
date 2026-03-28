package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const (
	ProviderNone       = ""
	ProviderDocker     = "docker"
	ProviderGVisor     = "gvisor"
	ProviderCloudflare = "cloudflare"

	defaultTimeout  = 30 * time.Second
	defaultMemoryMB = 256
	defaultCPUs     = 0.5
)

var languageImages = map[string]string{
	"python":     "python:3.12-slim",
	"javascript": "node:22-alpine",
	"bash":       "alpine:3.19",
	"go":         "golang:1.23-alpine",
}

type ExecuteResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Provider string
}

type CloudflareConfig struct {
	BridgeURL string
	AuthToken string
}

type executeRequest struct {
	Code     string         `json:"code"`
	Language string         `json:"language"`
	Input    map[string]any `json:"input"`
	Timeout  int            `json:"timeoutMs"`
}

type executeResponse struct {
	Output   map[string]any `json:"output"`
	Logs     []string       `json:"logs"`
	ExitCode int            `json:"exitCode"`
	Error    string         `json:"error,omitempty"`
}

func Available(provider string) bool {
	switch provider {
	case ProviderDocker:
		return exec.Command("docker", "info").Run() == nil
	case ProviderGVisor:
		if exec.Command("docker", "info").Run() != nil {
			return false
		}
		out, err := exec.Command("docker", "info", "--format", "{{json .Runtimes}}").Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "runsc")
	case ProviderCloudflare:
		return false
	case ProviderNone:
		return true
	default:
		return false
	}
}

func RunInContainer(ctx context.Context, provider, language, code string, input map[string]any) (*ExecuteResult, error) {
	image := languageImages[strings.ToLower(language)]
	if image == "" {
		image = "alpine:3.19"
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	args := []string{
		"run", "--rm",
		fmt.Sprintf("--memory=%dm", defaultMemoryMB),
		fmt.Sprintf("--cpus=%.2f", defaultCPUs),
		"--network=none",
		"-e", fmt.Sprintf("SUPERPLANE_INPUT=%s", inputJSON),
	}

	if provider == ProviderGVisor {
		args = append(args, "--runtime=runsc")
	}

	entrypoint, cmdArgs := buildCommand(language, code)
	if entrypoint != "" {
		args = append(args, "--entrypoint", entrypoint)
	}
	args = append(args, image)
	args = append(args, cmdArgs...)

	execCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("execution timed out after %s", defaultTimeout)
		} else {
			return nil, fmt.Errorf("docker run failed: %w", err)
		}
	}

	return &ExecuteResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
		Provider: provider,
	}, nil
}

func RunInCloudflare(ctx context.Context, cfg CloudflareConfig, language, code string, input map[string]any) (*ExecuteResult, error) {
	payload := executeRequest{
		Code:     code,
		Language: language,
		Input:    input,
		Timeout:  int(defaultTimeout.Milliseconds()),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BridgeURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bridge returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result executeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("sandbox error: %s", result.Error)
	}

	logs := strings.Join(result.Logs, "\n")
	return &ExecuteResult{
		Stdout:   fmt.Sprintf("%v", result.Output),
		Stderr:   logs,
		ExitCode: result.ExitCode,
		Duration: duration,
		Provider: ProviderCloudflare,
	}, nil
}

func buildCommand(language, code string) (string, []string) {
	switch strings.ToLower(language) {
	case "python":
		return "", []string{"python3", "-c", code}
	case "javascript":
		return "", []string{"node", "-e", code}
	case "bash":
		return "", []string{"sh", "-c", code}
	default:
		return "", []string{"sh", "-c", code}
	}
}

type WrappedComponent struct {
	inner    core.Component
	provider string
	cfConfig CloudflareConfig
}

func Wrap(c core.Component, provider string, cfConfig CloudflareConfig) core.Component {
	if provider == ProviderNone {
		return c
	}
	return &WrappedComponent{inner: c, provider: provider, cfConfig: cfConfig}
}

func (w *WrappedComponent) Name() string                  { return w.inner.Name() }
func (w *WrappedComponent) Label() string                 { return w.inner.Label() }
func (w *WrappedComponent) Description() string           { return w.inner.Description() }
func (w *WrappedComponent) Documentation() string         { return w.inner.Documentation() }
func (w *WrappedComponent) Icon() string                  { return w.inner.Icon() }
func (w *WrappedComponent) Color() string                 { return w.inner.Color() }
func (w *WrappedComponent) ExampleOutput() map[string]any { return w.inner.ExampleOutput() }

func (w *WrappedComponent) Configuration() []configuration.Field {
	return w.inner.Configuration()
}

func (w *WrappedComponent) OutputChannels(spec any) []core.OutputChannel {
	return w.inner.OutputChannels(spec)
}

func (w *WrappedComponent) Setup(ctx core.SetupContext) error {
	return w.inner.Setup(ctx)
}

func (w *WrappedComponent) Execute(ctx core.ExecutionContext) error {
	ctx.SandboxProvider = w.provider
	return w.inner.Execute(ctx)
}

func (w *WrappedComponent) Cancel(ctx core.ExecutionContext) error {
	return w.inner.Cancel(ctx)
}

func (w *WrappedComponent) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return w.inner.ProcessQueueItem(ctx)
}

func (w *WrappedComponent) Actions() []core.Action { return w.inner.Actions() }

func (w *WrappedComponent) HandleAction(ctx core.ActionContext) error {
	return w.inner.HandleAction(ctx)
}

func (w *WrappedComponent) HandleWebhook(ctx core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return w.inner.HandleWebhook(ctx)
}

func (w *WrappedComponent) Cleanup(ctx core.SetupContext) error {
	return w.inner.Cleanup(ctx)
}
