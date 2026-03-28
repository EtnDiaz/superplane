package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const cloudflareProviderName = "cloudflare"

type CloudflareProvider struct {
	BridgeURL string
	AuthToken string
}

type cloudflareSandboxRequest struct {
	Code     string         `json:"code"`
	Language string         `json:"language"`
	Input    map[string]any `json:"input"`
	Timeout  int            `json:"timeoutMs"`
}

type cloudflareSandboxResponse struct {
	Output   map[string]any `json:"output"`
	Logs     []string       `json:"logs"`
	ExitCode int            `json:"exitCode"`
	DurationMs int          `json:"durationMs"`
	Error    string         `json:"error,omitempty"`
}

func (p *CloudflareProvider) Name() string {
	return cloudflareProviderName
}

func (p *CloudflareProvider) Available() bool {
	return p.BridgeURL != "" && p.AuthToken != ""
}

func (p *CloudflareProvider) Run(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	start := time.Now()

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	payload := cloudflareSandboxRequest{
		Code:     req.Code,
		Language: req.Language,
		Input:    req.Input,
		Timeout:  int(timeout.Milliseconds()),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BridgeURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.AuthToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bridge returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result cloudflareSandboxResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("sandbox error: %s", result.Error)
	}

	output := result.Output
	if output == nil {
		output = map[string]any{}
	}

	return &SandboxResult{
		Output:   output,
		Logs:     result.Logs,
		ExitCode: result.ExitCode,
		Duration: time.Since(start),
		Provider: cloudflareProviderName,
	}, nil
}
