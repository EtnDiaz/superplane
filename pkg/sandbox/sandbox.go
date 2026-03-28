package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

	defaultTimeout = 30 * time.Second
)

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

type runnerExecuteRequest struct {
	Provider string         `json:"provider"`
	Language string         `json:"language"`
	Code     string         `json:"code"`
	Input    map[string]any `json:"input"`
	TimeoutS int            `json:"timeoutSeconds"`
}

type runnerExecuteResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Duration int64  `json:"durationMs"`
}

type runnerStatusResponse struct {
	Docker bool   `json:"docker"`
	GVisor bool   `json:"gvisor"`
	Reason string `json:"reason,omitempty"`
}

type cfExecuteRequest struct {
	Code     string         `json:"code"`
	Language string         `json:"language"`
	Input    map[string]any `json:"input"`
	Timeout  int            `json:"timeoutMs"`
}

type cfExecuteResponse struct {
	Output   map[string]any `json:"output"`
	Logs     []string       `json:"logs"`
	ExitCode int            `json:"exitCode"`
	Error    string         `json:"error,omitempty"`
}

func runnerURL() string {
	u := os.Getenv("SANDBOX_RUNNER_URL")
	if u == "" {
		return "http://localhost:8888"
	}
	return u
}

func Available(ctx context.Context, provider string) (bool, string) {
	switch provider {
	case ProviderNone:
		return true, ""
	case ProviderDocker:
		return checkRunnerDocker(ctx)
	case ProviderGVisor:
		return checkRunnerGVisor(ctx)
	case ProviderCloudflare:
		return false, "configure Bridge Worker URL and auth token in canvas settings"
	default:
		return false, fmt.Sprintf("unknown provider: %s", provider)
	}
}

func checkRunnerDocker(ctx context.Context) (bool, string) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, runnerURL()+"/status", nil)
	if err != nil {
		return false, "sandbox-runner unreachable: " + err.Error()
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "sandbox-runner unreachable: " + err.Error()
	}
	defer resp.Body.Close()

	var status runnerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, "sandbox-runner returned invalid response"
	}

	if !status.Docker {
		reason := status.Reason
		if reason == "" {
			reason = "Docker not available on sandbox-runner"
		}
		return false, reason
	}

	return true, ""
}

func checkRunnerGVisor(ctx context.Context) (bool, string) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, runnerURL()+"/status", nil)
	if err != nil {
		return false, "sandbox-runner unreachable: " + err.Error()
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "sandbox-runner unreachable: " + err.Error()
	}
	defer resp.Body.Close()

	var status runnerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, "sandbox-runner returned invalid response"
	}

	if !status.GVisor {
		reason := status.Reason
		if reason == "" {
			reason = "gVisor not available on sandbox-runner"
		}
		return false, reason
	}

	return true, ""
}

func RunInGVisor(ctx context.Context, language, code string, input map[string]any, timeoutS int) (*ExecuteResult, error) {
	return callRunner(ctx, ProviderGVisor, language, code, input, timeoutS)
}

func RunInDocker(ctx context.Context, language, code string, input map[string]any, timeoutS int) (*ExecuteResult, error) {
	return callRunner(ctx, ProviderDocker, language, code, input, timeoutS)
}

func callRunner(ctx context.Context, provider, language, code string, input map[string]any, timeoutS int) (*ExecuteResult, error) {
	payload := runnerExecuteRequest{
		Provider: provider,
		Language: language,
		Code:     code,
		Input:    input,
		TimeoutS: timeoutS,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	callTimeout := defaultTimeout
	if timeoutS > 0 {
		callTimeout = time.Duration(timeoutS)*time.Second + 5*time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, runnerURL()+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sandbox-runner request failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sandbox-runner returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result runnerExecuteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ExecuteResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: duration,
		Provider: provider,
	}, nil
}

func RunInCloudflare(ctx context.Context, cfg CloudflareConfig, language, code string, input map[string]any) (*ExecuteResult, error) {
	payload := cfExecuteRequest{
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

	var result cfExecuteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("sandbox error: %s", result.Error)
	}

	return &ExecuteResult{
		Stdout:   fmt.Sprintf("%v", result.Output),
		Stderr:   strings.Join(result.Logs, "\n"),
		ExitCode: result.ExitCode,
		Duration: duration,
		Provider: ProviderCloudflare,
	}, nil
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
