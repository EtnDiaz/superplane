package sandbox

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/core"
)

func decodeConfig(input any, output any) error {
	if err := mapstructure.Decode(input, output); err != nil {
		return fmt.Errorf("failed to decode configuration: %w", err)
	}
	return nil
}

func readIntegrationConfig(ctx core.IntegrationContext) (Configuration, error) {
	providerBytes, err := ctx.GetConfig("provider")
	if err != nil {
		return Configuration{}, fmt.Errorf("failed to read provider: %w", err)
	}

	config := Configuration{
		Provider: string(providerBytes),
	}

	bridgeURLBytes, err := ctx.GetConfig("bridgeUrl")
	if err == nil {
		config.BridgeURL = string(bridgeURLBytes)
	}

	authTokenBytes, err := ctx.GetConfig("authToken")
	if err == nil {
		config.AuthToken = string(authTokenBytes)
	}

	return config, nil
}

func validateConfig(config Configuration) error {
	switch config.Provider {
	case "gvisor", "docker":
		return nil
	case "cloudflare":
		if config.BridgeURL == "" {
			return fmt.Errorf("bridgeUrl is required for Cloudflare provider")
		}
		if config.AuthToken == "" {
			return fmt.Errorf("authToken is required for Cloudflare provider")
		}
		return nil
	case "":
		return fmt.Errorf("provider is required")
	default:
		return fmt.Errorf("unknown provider %q: must be one of gvisor, docker, cloudflare", config.Provider)
	}
}

func buildProvider(config Configuration) (SandboxProvider, error) {
	switch config.Provider {
	case "gvisor":
		return &GVisorProvider{}, nil
	case "docker":
		return &DockerProvider{}, nil
	case "cloudflare":
		return &CloudflareProvider{
			BridgeURL: config.BridgeURL,
			AuthToken: config.AuthToken,
		}, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", config.Provider)
	}
}
