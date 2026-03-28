package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/registry"
	sandboxpkg "github.com/superplanehq/superplane/pkg/sandbox"
)

func init() {
	registry.RegisterComponent("sandbox", &Sandbox{})
}

type Sandbox struct{}

type Spec struct {
	Provider   string `mapstructure:"provider"`
	Language   string `mapstructure:"language"`
	Code       string `mapstructure:"code"`
	TimeoutS   int    `mapstructure:"timeoutSeconds"`
	MemoryMB   int    `mapstructure:"memoryMb"`
	BridgeURL  string `mapstructure:"bridgeUrl"`
	AuthToken  string `mapstructure:"authToken"`
}

func (c *Sandbox) Name() string        { return "sandbox" }
func (c *Sandbox) Label() string       { return "Sandbox" }
func (c *Sandbox) Icon() string        { return "shield" }
func (c *Sandbox) Color() string       { return "gray" }
func (c *Sandbox) Description() string { return "Execute code in an isolated sandbox" }
func (c *Sandbox) Documentation() string {
	return `Execute code in an isolated sandbox environment.

Your code receives the previous node's output as ` + "`SUPERPLANE_INPUT`" + ` (JSON env var).
Print a JSON object to stdout — that becomes this node's output.

**Providers:**
- ` + "`docker`" + ` — Docker container (local, requires sandbox-runner sidecar)
- ` + "`gvisor`" + ` — gVisor kernel isolation (local, requires runsc runtime)
- ` + "`cloudflare`" + ` — Cloudflare Dynamic Workers (remote, requires Bridge Worker)

**Example (Python):**
` + "```python" + `
import os, json
data = json.loads(os.environ.get("SUPERPLANE_INPUT", "{}"))
print(json.dumps({"processed": True, "count": len(data)}))
` + "```"
}

func (c *Sandbox) ExampleOutput() map[string]any {
	return map[string]any{
		"output":     map[string]any{"result": "ok"},
		"logs":       []string{},
		"exitCode":   0,
		"durationMs": 312,
		"provider":   "docker",
	}
}

func (c *Sandbox) OutputChannels(_ any) []core.OutputChannel {
	return []core.OutputChannel{
		{Name: "default", Label: "Default"},
	}
}

func (c *Sandbox) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:     "provider",
			Label:    "Provider",
			Type:     configuration.FieldTypeSelect,
			Required: true,
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Docker (local)", Value: "docker"},
						{Label: "gVisor (local, most secure)", Value: "gvisor"},
						{Label: "Cloudflare Workers (remote)", Value: "cloudflare"},
					},
				},
			},
		},
		{
			Name:     "language",
			Label:    "Language",
			Type:     configuration.FieldTypeSelect,
			Required: true,
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Python", Value: "python"},
						{Label: "JavaScript", Value: "javascript"},
						{Label: "Bash", Value: "bash"},
						{Label: "Ruby", Value: "ruby"},
					},
				},
			},
		},
		{
			Name:        "code",
			Label:       "Code",
			Type:        configuration.FieldTypeText,
			Required:    true,
			Description: "Code to execute. Print JSON to stdout to pass output to the next step.",
			Placeholder: "import os, json\ndata = json.loads(os.environ.get('SUPERPLANE_INPUT', '{}'))\nprint(json.dumps({'result': 'ok'}))",
		},
		{
			Name:        "bridgeUrl",
			Label:       "Bridge Worker URL",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Cloudflare Bridge Worker URL (required for cloudflare provider)",
			Placeholder: "https://superplane-sandbox-bridge.example.workers.dev",
			VisibilityConditions: []configuration.VisibilityCondition{
				{Field: "provider", Values: []string{"cloudflare"}},
			},
		},
		{
			Name:        "authToken",
			Label:       "Auth Token",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Sensitive:   true,
			Description: "Auth token for the Cloudflare Bridge Worker",
			VisibilityConditions: []configuration.VisibilityCondition{
				{Field: "provider", Values: []string{"cloudflare"}},
			},
		},
		{
			Name:        "timeoutSeconds",
			Label:       "Timeout (seconds)",
			Type:        configuration.FieldTypeNumber,
			Required:    false,
			Description: "Max execution time (default: 30)",
		},
	}
}

func (c *Sandbox) Setup(ctx core.SetupContext) error {
	var spec Spec
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	if spec.Provider == "" {
		return errors.New("provider is required")
	}
	if spec.Language == "" {
		return errors.New("language is required")
	}
	if spec.Code == "" {
		return errors.New("code is required")
	}
	if spec.Provider == sandboxpkg.ProviderCloudflare && spec.BridgeURL == "" {
		return errors.New("bridgeUrl is required for cloudflare provider")
	}
	return nil
}

func (c *Sandbox) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (c *Sandbox) Execute(ctx core.ExecutionContext) error {
	var spec Spec
	if err := mapstructure.Decode(ctx.Configuration, &spec); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	input := extractInput(ctx.Data)

	var result *sandboxpkg.ExecuteResult
	var err error

	switch spec.Provider {
	case sandboxpkg.ProviderDocker, sandboxpkg.ProviderGVisor:
		result, err = sandboxpkg.RunInGVisor(context.Background(), spec.Language, spec.Code, input, spec.TimeoutS)
		if spec.Provider == sandboxpkg.ProviderDocker {
			result, err = sandboxpkg.RunInDocker(context.Background(), spec.Language, spec.Code, input, spec.TimeoutS)
		}
	case sandboxpkg.ProviderCloudflare:
		bridgeURL := spec.BridgeURL
		authToken := spec.AuthToken
		if bridgeURL == "" {
			return errors.New("bridgeUrl is required for cloudflare provider")
		}
		result, err = sandboxpkg.RunInCloudflare(context.Background(), sandboxpkg.CloudflareConfig{
			BridgeURL: bridgeURL,
			AuthToken: authToken,
		}, spec.Language, spec.Code, input)
	default:
		return fmt.Errorf("unknown provider: %s", spec.Provider)
	}

	if err != nil {
		return fmt.Errorf("sandbox execution failed: %w", err)
	}

	if result.ExitCode != 0 {
		return ctx.ExecutionState.Fail("non_zero_exit",
			fmt.Sprintf("sandbox exited with code %d: %s", result.ExitCode, result.Stderr))
	}

	output := parseOutput(result.Stdout)
	payload := map[string]any{
		"output":     output,
		"logs":       splitLines(result.Stderr),
		"exitCode":   result.ExitCode,
		"durationMs": result.Duration.Milliseconds(),
		"provider":   result.Provider,
	}

	return ctx.ExecutionState.Emit("default", "sandbox.result", []any{payload})
}

func (c *Sandbox) Cancel(_ core.ExecutionContext) error    { return nil }
func (c *Sandbox) Cleanup(_ core.SetupContext) error       { return nil }
func (c *Sandbox) Actions() []core.Action                  { return nil }
func (c *Sandbox) HandleAction(_ core.ActionContext) error { return nil }
func (c *Sandbox) HandleWebhook(_ core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusNotFound, nil, nil
}

func extractInput(data any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	if m, ok := data.(map[string]any); ok {
		return m
	}
	return map[string]any{"data": data}
}

func parseOutput(stdout string) map[string]any {
	trimmed := []byte(stdout)
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\n' || trimmed[0] == '\r') {
		trimmed = trimmed[1:]
	}
	for len(trimmed) > 0 && (trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\n' || trimmed[len(trimmed)-1] == '\r') {
		trimmed = trimmed[:len(trimmed)-1]
	}
	var out map[string]any
	if err := json.Unmarshal(trimmed, &out); err == nil {
		return out
	}
	return map[string]any{"raw": string(trimmed)}
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	var lines []string
	for _, line := range splitString(s, '\n') {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
