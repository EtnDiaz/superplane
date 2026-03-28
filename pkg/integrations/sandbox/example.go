package sandbox

import (
	_ "embed"
	"sync"

	"github.com/superplanehq/superplane/pkg/utils"
)

//go:embed example_output_run_sandbox.json
var exampleOutputRunSandboxBytes []byte

var exampleOutputRunSandboxOnce sync.Once
var exampleOutputRunSandbox map[string]any

func (c *RunSandbox) ExampleOutput() map[string]any {
	return utils.UnmarshalEmbeddedJSON(&exampleOutputRunSandboxOnce, exampleOutputRunSandboxBytes, &exampleOutputRunSandbox)
}
