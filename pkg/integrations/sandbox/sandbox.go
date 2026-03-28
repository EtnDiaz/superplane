package sandbox

import (
	"github.com/superplanehq/superplane/pkg/configuration"
	"github.com/superplanehq/superplane/pkg/core"
	"github.com/superplanehq/superplane/pkg/registry"
)

func init() {
	registry.RegisterIntegration("sandbox", &Sandbox{})
}

type Sandbox struct{}

type Configuration struct {
	Provider  string `json:"provider"`
	BridgeURL string `json:"bridgeUrl"`
	AuthToken string `json:"authToken"`
}

func (s *Sandbox) Name() string {
	return "sandbox"
}

func (s *Sandbox) Label() string {
	return "Sandbox"
}

func (s *Sandbox) Icon() string {
	return "shield"
}

func (s *Sandbox) Description() string {
	return "Execute arbitrary code in an isolated sandbox (gVisor, Docker, or Cloudflare Workers)"
}

func (s *Sandbox) Instructions() string {
	return `## Sandbox Integration

Choose a sandbox provider:

### gVisor (local, most secure)
Requires Docker with gVisor runtime installed on the SuperPlane host.
- Install: https://gvisor.dev/docs/user_guide/install/
- No additional configuration needed — select **gVisor** as provider.

### Docker (local, easy setup)
Requires Docker on the SuperPlane host. Less isolated than gVisor but simpler to set up.
- No additional configuration needed — select **Docker** as provider.

### Cloudflare Workers (remote, zero infrastructure)
Requires a deployed Cloudflare Bridge Worker and an auth token.
- **Bridge URL**: The URL of your deployed Bridge Worker (e.g. https://sandbox-bridge.yourname.workers.dev)
- **Auth Token**: A secret token you configure in your Bridge Worker to authenticate requests from SuperPlane`
}

func (s *Sandbox) Configuration() []configuration.Field {
	return []configuration.Field{
		{
			Name:        "provider",
			Label:       "Provider",
			Type:        configuration.FieldTypeSelect,
			Required:    true,
			Description: "The sandbox backend to use for code execution",
			TypeOptions: &configuration.TypeOptions{
				Select: &configuration.SelectTypeOptions{
					Options: []configuration.FieldOption{
						{Label: "gVisor (local, most secure)", Value: "gvisor"},
						{Label: "Docker (local, easy setup)", Value: "docker"},
						{Label: "Cloudflare Workers (remote, zero infra)", Value: "cloudflare"},
					},
				},
			},
		},
		{
			Name:        "bridgeUrl",
			Label:       "Bridge URL",
			Type:        configuration.FieldTypeString,
			Required:    false,
			Description: "URL of your Cloudflare Bridge Worker (e.g. https://sandbox-bridge.example.workers.dev)",
			Placeholder: "https://sandbox-bridge.example.workers.dev",
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
			Description: "Secret token to authenticate requests to the Bridge Worker",
			VisibilityConditions: []configuration.VisibilityCondition{
				{Field: "provider", Values: []string{"cloudflare"}},
			},
		},
	}
}

func (s *Sandbox) Components() []core.Component {
	return []core.Component{
		&RunSandbox{},
	}
}

func (s *Sandbox) Triggers() []core.Trigger {
	return []core.Trigger{}
}

func (s *Sandbox) Sync(ctx core.SyncContext) error {
	config := Configuration{}
	if err := decodeConfig(ctx.Configuration, &config); err != nil {
		return err
	}

	if err := validateConfig(config); err != nil {
		return err
	}

	ctx.Integration.Ready()
	return nil
}

func (s *Sandbox) Cleanup(ctx core.IntegrationCleanupContext) error {
	return nil
}

func (s *Sandbox) ListResources(resourceType string, ctx core.ListResourcesContext) ([]core.IntegrationResource, error) {
	return []core.IntegrationResource{}, nil
}

func (s *Sandbox) HandleRequest(ctx core.HTTPRequestContext) {}

func (s *Sandbox) Actions() []core.Action {
	return []core.Action{}
}

func (s *Sandbox) HandleAction(ctx core.IntegrationActionContext) error {
	return nil
}
