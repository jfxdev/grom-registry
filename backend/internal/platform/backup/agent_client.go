package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type AgentClient struct {
	socketPath string
	client     *http.Client
}

func NewAgentClient(socketPath string) *AgentClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &AgentClient{
		socketPath: socketPath,
		client:     &http.Client{Transport: transport, Timeout: 0},
	}
}

func (client *AgentClient) List(ctx context.Context) ([]Summary, error) {
	var result []Summary
	if err := client.doJSON(ctx, http.MethodGet, "/v1/backups", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (client *AgentClient) Create(ctx context.Context, request AgentCreateRequest) (Summary, error) {
	var result Summary
	if err := client.doJSON(ctx, http.MethodPost, "/v1/backups", request, &result); err != nil {
		return Summary{}, err
	}
	return result, nil
}

func (client *AgentClient) Download(ctx context.Context, backupID string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://grom-agent/v1/backups/"+backupID+"/bundle", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("contact backup agent: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("backup agent returned status %d", response.StatusCode)
	}
	return response, nil
}

func (client *AgentClient) Delete(ctx context.Context, backupID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://grom-agent/v1/backups/"+backupID, nil)
	if err != nil {
		return err
	}
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("contact backup agent: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("backup agent returned status %d", response.StatusCode)
	}
	return nil
}

func (client *AgentClient) Available(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := client.List(checkCtx)
	return err == nil
}

func (client *AgentClient) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://grom-agent"+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("contact backup agent at configured socket: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("backup agent returned status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode backup agent response: %w", err)
	}
	return nil
}
