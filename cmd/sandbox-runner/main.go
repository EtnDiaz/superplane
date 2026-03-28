package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	"go":         "golang:1.23-alpine",
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
	log.Infof("sandbox-runner listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleStatus(w http.ResponseWriter, _ *http.Request) {
	dockerOK := exec.Command("docker", "info").Run() == nil

	gvisorOK := false
	reason := ""
	if dockerOK {
		out, err := exec.Command("docker", "info", "--format", "{{json .Runtimes}}").Output()
		if err == nil && strings.Contains(string(out), "runsc") {
			gvisorOK = true
		} else {
			reason = "gVisor runtime (runsc) not found in Docker runtimes"
		}
	} else {
		reason = "Docker daemon not accessible via socket"
	}

	resp := statusResponse{
		Docker: dockerOK,
		GVisor: gvisorOK,
		Reason: reason,
	}

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

	result, err := runInDocker(r.Context(), req.Provider, req.Language, req.Code, req.Input, timeout, memMB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func runInDocker(ctx context.Context, provider, language, code string, input map[string]any, timeout time.Duration, memMB int) (*executeResponse, error) {
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
		fmt.Sprintf("--memory=%dm", memMB),
		fmt.Sprintf("--cpus=%.2f", defaultCPUs),
		"--network=none",
		"-e", fmt.Sprintf("SUPERPLANE_INPUT=%s", inputJSON),
	}

	if provider == "gvisor" {
		args = append(args, "--runtime=runsc")
	}

	entrypoint, cmdArgs := buildCommand(language, code)
	if entrypoint != "" {
		args = append(args, "--entrypoint", entrypoint)
	}
	args = append(args, image)
	args = append(args, cmdArgs...)

	execCtx, cancel := context.WithTimeout(ctx, timeout)
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
			return nil, fmt.Errorf("execution timed out after %s", timeout)
		} else {
			return nil, fmt.Errorf("docker run failed: %w", err)
		}
	}

	return &executeResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration.Milliseconds(),
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
	case "ruby":
		return "", []string{"ruby", "-e", code}
	default:
		return "", []string{"sh", "-c", code}
	}
}
