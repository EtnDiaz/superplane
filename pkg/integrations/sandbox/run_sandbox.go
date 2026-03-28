package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
)

const sandboxResultPayloadType = "sandbox.result"

type RunSandbox struct{}

type RunSandboxSpec struct {
	Language string  `json:"language"`
	Code     string  `json:"code"`
	Image    string  `json:"image"`
	TimeoutS *int    `json:"timeoutSeconds"`
	MemoryMB *int    `json:"memoryMb"`
	CPUs     *string `json:"cpus"`
}

func (c *RunSandbox) Name() string {
	return "sandbox.run"
}

func (c *RunSandbox) Label() string {
	return "Run Sandbox"
}

func (c *RunSandbox) Description() string {
	return "Execute code in an isolated sandbox and return the output"
}

func (c *RunSandbox) Documentation() string {
	return `The Run Sandbox component executes arbitrary code in a secure, isolated environment.

## Input

The previous node's output is available as JSON in the ` + "`SUPERPLANE_INPUT`" + ` environment variable.

## Output

Your code should print a JSON object to stdout. That object becomes the component's output and is available to downstream nodes.

If stdout is not valid JSON, it is captured as ` + "`{\"raw\": \"...\"}`" + `.

## Examples

**Python** — transform data from the previous step:
` + "```python" + `
import os, json
data = json.loads(os.environ.get("SUPERPLANE_INPUT", "{}"))
result = {"count": len(data.get("items", [])), "status": "ok"}
print(json.dumps(result))
` + "```" + `

**Bash** — run a shell command:
` + "```bash" + `
echo '{"hostname": "'"$(hostname)"'", "uptime": "'"$(uptime -p)"'"}'
` + "```" + `

**JavaScript** — call an API:
` + "```javascript" + `
const input = JSON.parse(process.env.SUPERPLANE_INPUT || "{}");
const result = { processed: true, items: input.items?.length ?? 0 };
console.log(JSON.stringify(result));
` + "```"
}

func (c *RunSandbox) Icon() string {
	return "terminal"
}

func (c *RunSandbox) Color() string {
	return "gray"
}

func (c *RunSandbox) OutputChannels(_ any) []core.OutputChannel {
	return []core.OutputChannel{
		{Name: core.DefaultOutputChannel.Name, Label: core.DefaultOutputChannel.Label},
	}
}

func (c *RunSandbox) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:     "language",
			Label:    "Language",
			Type:     configuration.FieldTypeSelect,
			Required: true,
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "Python", Value: "python"},
						{Label: "JavaScript (Node.js)", Value: "javascript"},
						{Label: "Bash", Value: "bash"},
						{Label: "Go", Value: "go"},
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
			Description: "Code to execute. Print a JSON object to stdout to pass data to the next step.",
			Placeholder: `import os, json
data = json.loads(os.environ.get("SUPERPLANE_INPUT", "{}"))
print(json.dumps({"result": "ok"}))`,
		},
		{
			Name:        "image",
			Label:       "Docker Image",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "Custom Docker image to use (overrides the default for the selected language). Not applicable for Cloudflare provider.",
			Placeholder: "python:3.12-slim",
		},
		{
			Name:        "timeoutSeconds",
			Label:       "Timeout (seconds)",
			Type:        configuration.FieldTypeNumber,
			Required:    false,
			Description: "Maximum execution time in seconds (default: 30)",
			TypeOptions: &configuration.TypeOptions{
				Number: &configuration.NumberTypeOptions{
					Min: func() *int { v := 1; return &v }(),
					Max: func() *int { v := 300; return &v }(),
				},
			},
		},
		{
			Name:        "memoryMb",
			Label:       "Memory (MB)",
			Type:        configuration.FieldTypeNumber,
			Required:    false,
			Description: "Memory limit in megabytes (default: 256). Not applicable for Cloudflare provider.",
			TypeOptions: &configuration.TypeOptions{
				Number: &configuration.NumberTypeOptions{
					Min: func() *int { v := 32; return &v }(),
					Max: func() *int { v := 4096; return &v }(),
				},
			},
		},
	}
}

func (c *RunSandbox) Setup(ctx core.SetupContext) error {
	spec := RunSandboxSpec{}
	if err := decodeConfig(ctx.Configuration, &spec); err != nil {
		return err
	}

	if spec.Language == "" {
		return errors.New("language is required")
	}

	if spec.Code == "" {
		return errors.New("code is required")
	}

	if spec.TimeoutS != nil && (*spec.TimeoutS < 1 || *spec.TimeoutS > 300) {
		return errors.New("timeoutSeconds must be between 1 and 300")
	}

	if spec.MemoryMB != nil && (*spec.MemoryMB < 32 || *spec.MemoryMB > 4096) {
		return errors.New("memoryMb must be between 32 and 4096")
	}

	return nil
}

func (c *RunSandbox) Execute(ctx core.ExecutionContext) error {
	spec := RunSandboxSpec{}
	if err := decodeConfig(ctx.Configuration, &spec); err != nil {
		return err
	}

	integrationConfig, err := readIntegrationConfig(ctx.Integration)
	if err != nil {
		return fmt.Errorf("failed to read integration config: %w", err)
	}

	provider, err := buildProvider(integrationConfig)
	if err != nil {
		return fmt.Errorf("failed to build provider: %w", err)
	}

	if !provider.Available() {
		return fmt.Errorf("sandbox provider %q is not available in this environment", provider.Name())
	}

	input := extractInput(ctx)

	timeout := 30 * time.Second
	if spec.TimeoutS != nil {
		timeout = time.Duration(*spec.TimeoutS) * time.Second
	}

	memMB := 0
	if spec.MemoryMB != nil {
		memMB = *spec.MemoryMB
	}

	req := SandboxRequest{
		Code:     spec.Code,
		Language: spec.Language,
		Image:    spec.Image,
		Input:    input,
		Timeout:  timeout,
		MemoryMB: memMB,
	}

	result, err := provider.Run(context.Background(), req)
	if err != nil {
		return fmt.Errorf("sandbox execution failed: %w", err)
	}

	if result.ExitCode != 0 {
		logs := joinLogs(result.Logs)
		if err := ctx.ExecutionState.Fail("non_zero_exit", fmt.Sprintf("sandbox exited with code %d: %s", result.ExitCode, logs)); err != nil {
			return fmt.Errorf("failed to mark execution as failed: %w", err)
		}
		return nil
	}

	payload := map[string]any{
		"output":     result.Output,
		"logs":       result.Logs,
		"exitCode":   result.ExitCode,
		"durationMs": result.Duration.Milliseconds(),
		"provider":   result.Provider,
	}

	return ctx.ExecutionState.Emit(
		core.DefaultOutputChannel.Name,
		sandboxResultPayloadType,
		[]any{payload},
	)
}

func (c *RunSandbox) Cancel(_ core.ExecutionContext) error {
	return nil
}

func (c *RunSandbox) ProcessQueueItem(ctx core.ProcessQueueContext) (*uuid.UUID, error) {
	return ctx.DefaultProcessing()
}

func (c *RunSandbox) Actions() []core.Action {
	return []core.Action{}
}

func (c *RunSandbox) HandleAction(_ core.ActionContext) error {
	return nil
}

func (c *RunSandbox) HandleWebhook(_ core.WebhookRequestContext) (int, *core.WebhookResponseBody, error) {
	return http.StatusOK, nil, nil
}

func (c *RunSandbox) Cleanup(_ core.SetupContext) error {
	return nil
}

func extractInput(ctx core.ExecutionContext) map[string]any {
	if ctx.Data == nil {
		return map[string]any{}
	}

	if m, ok := ctx.Data.(map[string]any); ok {
		return m
	}

	return map[string]any{"data": ctx.Data}
}

func joinLogs(logs []string) string {
	if len(logs) == 0 {
		return ""
	}

	result := ""
	for i, l := range logs {
		if i > 0 {
			result += "; "
		}
		result += l
	}
	return result
}
