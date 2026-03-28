package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	defaultPort     = "8888"
	defaultTimeout  = 30 * time.Second
	defaultMemoryMB = 256
	defaultCPUs     = 0.5
)

var languageImages = map[string]string{
	"python":     "python:3.12-slim",
	"javascript": "node:22-alpine",
	"bash":       "alpine:3.19",
	"ruby":       "ruby:3.3-alpine",
}

type executeRequest struct {
	Provider string         `json:"provider"`
	Language string         `json:"language"`
	Code     string         `json:"code"`
	Input    map[string]any `json:"input"`
	TimeoutS int            `json:"timeoutSeconds"`
	MemoryMB int            `json:"memoryMb"`
}

type executeResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Duration int64  `json:"durationMs"`
}

type statusResponse struct {
	Docker bool   `json:"docker"`
	GVisor bool   `json:"gvisor"`
	Reason string `json:"reason,omitempty"`
}

func dockerSocketPath() string {
	if _, err := os.Stat("/run/host-services/docker.proxy.sock"); err == nil {
		return "/run/host-services/docker.proxy.sock"
	}
	return "/var/run/docker.sock"
}

func newDockerClient() *http.Client {
	sock := dockerSocketPath()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 120 * time.Second,
	}
}

func dockerGet(ctx context.Context, path string) ([]byte, int, error) {
	client := newDockerClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func dockerPost(ctx context.Context, path string, payload any) ([]byte, int, error) {
	client := newDockerClient()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker"+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, err
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/execute", handleExecute)

	addr := ":" + port
	log.Infof("sandbox-runner listening on %s (docker socket: %s)", addr, dockerSocketPath())
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, code, err := dockerGet(ctx, "/v1.41/info")
	dockerOK := err == nil && code == http.StatusOK

	gvisorOK := false
	reason := ""
	if dockerOK {
		body, _, err := dockerGet(ctx, "/v1.41/info")
		if err == nil {
			var info struct {
				Runtimes map[string]any `json:"Runtimes"`
			}
			if json.Unmarshal(body, &info) == nil {
				_, gvisorOK = info.Runtimes["runsc"]
				if !gvisorOK {
					reason = "gVisor runtime (runsc) not found in Docker runtimes — install from gvisor.dev"
				}
			}
		}
	} else {
		reason = fmt.Sprintf("Docker daemon not accessible via socket (%s)", dockerSocketPath())
	}

	resp := statusResponse{Docker: dockerOK, GVisor: gvisorOK, Reason: reason}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	timeout := defaultTimeout
	if req.TimeoutS > 0 {
		timeout = time.Duration(req.TimeoutS) * time.Second
	}
	memMB := defaultMemoryMB
	if req.MemoryMB > 0 {
		memMB = req.MemoryMB
	}

	result, err := runInDockerAPI(r.Context(), req.Provider, req.Language, req.Code, req.Input, timeout, memMB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func runInDockerAPI(ctx context.Context, provider, language, code string, input map[string]any, timeout time.Duration, memMB int) (*executeResponse, error) {
	image := languageImages[strings.ToLower(language)]
	if image == "" {
		image = "alpine:3.19"
	}

	inputJSON, _ := json.Marshal(input)
	cmd, args := buildCommand(language, code)

	pullCtx, pullCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pullCancel()
	_, _, _ = dockerPost(pullCtx, "/v1.41/images/create?fromImage="+image, nil)

	containerConfig := map[string]any{
		"Image":      image,
		"Cmd":        append([]string{cmd}, args...),
		"Env":        []string{fmt.Sprintf("SUPERPLANE_INPUT=%s", inputJSON)},
		"NetworkDisabled": true,
		"HostConfig": map[string]any{
			"Memory":    int64(memMB) * 1024 * 1024,
			"NanoCPUs":  int64(defaultCPUs * 1e9),
			"AutoRemove": false,
			"Runtime":   runtimeForProvider(provider),
		},
	}

	// Create container
	createBody, createStatus, err := dockerPost(ctx, "/v1.41/containers/create", containerConfig)
	if err != nil || createStatus != http.StatusCreated {
		return nil, fmt.Errorf("create container failed (status %d): %s %v", createStatus, string(createBody), err)
	}

	var createResp struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(createBody, &createResp); err != nil || createResp.ID == "" {
		return nil, fmt.Errorf("parse create response: %w — %s", err, string(createBody))
	}
	containerID := createResp.ID

	// Ensure cleanup
	defer func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		_, _, _ = dockerPost(rmCtx, "/v1.41/containers/"+containerID+"/kill", nil)
		client := newDockerClient()
		req, _ := http.NewRequestWithContext(rmCtx, http.MethodDelete, "http://docker/v1.41/containers/"+containerID+"?force=true&v=true", nil)
		if req != nil {
			resp, _ := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
		}
	}()

	start := time.Now()
	_, startStatus, err := dockerPost(ctx, "/v1.41/containers/"+containerID+"/start", nil)
	if err != nil || startStatus != http.StatusNoContent {
		return nil, fmt.Errorf("start container failed (status %d): %v", startStatus, err)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, timeout)
	defer waitCancel()

	waitBody, _, err := dockerPost(waitCtx, "/v1.41/containers/"+containerID+"/wait?condition=not-running", nil)
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("execution timed out after %s", timeout)
		}
		return nil, fmt.Errorf("wait failed: %w", err)
	}

	var waitResp struct {
		StatusCode int `json:"StatusCode"`
	}
	json.Unmarshal(waitBody, &waitResp)
	exitCode = waitResp.StatusCode

	stdout, stderr := getLogs(ctx, containerID)

	return &executeResponse{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: duration.Milliseconds(),
	}, nil
}

func getLogs(ctx context.Context, containerID string) (string, string) {
	client := newDockerClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker/v1.41/containers/"+containerID+"/logs?stdout=true&stderr=true",
		nil)
	if err != nil {
		return "", ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ""
	}

	var stdout, stderr strings.Builder
	for len(raw) >= 8 {
		streamType := raw[0]
		size := int(raw[4])<<24 | int(raw[5])<<16 | int(raw[6])<<8 | int(raw[7])
		if len(raw) < 8+size {
			break
		}
		line := string(raw[8 : 8+size])
		switch streamType {
		case 1:
			stdout.WriteString(line)
		case 2:
			stderr.WriteString(line)
		}
		raw = raw[8+size:]
	}

	return stdout.String(), stderr.String()
}

func runtimeForProvider(provider string) string {
	if provider == "gvisor" {
		return "runsc"
	}
	return ""
}

func buildCommand(language, code string) (string, []string) {
	switch strings.ToLower(language) {
	case "python":
		return "python3", []string{"-c", code}
	case "javascript":
		return "node", []string{"-e", code}
	case "ruby":
		return "ruby", []string{"-e", code}
	default:
		return "sh", []string{"-c", code}
	}
}


