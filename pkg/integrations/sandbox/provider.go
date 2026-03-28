package sandbox

import (
	"context"
	"time"
)

// SandboxProvider is the interface that all sandbox execution backends implement.
type SandboxProvider interface {
	// Name returns the unique identifier for this provider.
	Name() string

	// Available returns true if this provider can be used in the current environment.
	Available() bool

	// Run executes the given code in an isolated sandbox and returns the result.
	Run(ctx context.Context, req SandboxRequest) (*SandboxResult, error)
}

// SandboxRequest describes the code to run and the input data to pass to it.
type SandboxRequest struct {
	// Code is the source code to execute.
	Code string

	// Language is the programming language of the code (e.g. "python", "javascript", "bash").
	Language string

	// Image is an optional Docker image override. If empty, a default image for the
	// language is used (only applicable for container-based providers like gVisor).
	Image string

	// Input is the JSON-serializable data passed to the sandbox as input.
	// It is available as the SUPERPLANE_INPUT environment variable (JSON-encoded).
	Input map[string]any

	// Timeout is the maximum time allowed for execution.
	Timeout time.Duration

	// MemoryMB is the memory limit in megabytes (0 = provider default).
	MemoryMB int

	// CPUs is the CPU limit (0 = provider default, e.g. 0.5 = half a core).
	CPUs float64
}

// SandboxResult holds the output of a sandbox execution.
type SandboxResult struct {
	// Output is the parsed JSON output written to stdout by the sandbox.
	// If stdout is not valid JSON, it is placed under the "raw" key.
	Output map[string]any

	// Logs contains all lines written to stderr.
	Logs []string

	// ExitCode is the process exit code (0 = success).
	ExitCode int

	// Duration is the wall-clock time of the execution.
	Duration time.Duration

	// Provider is the name of the provider that ran this execution.
	Provider string
}
