package registrymaintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Client struct{ client *http.Client }

func NewClient(socketPath string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath)
	}}
	return &Client{client: &http.Client{Transport: transport}}
}
func (c *Client) Storage(ctx context.Context) (Storage, error) {
	var result Storage
	return result, c.do(ctx, http.MethodGet, "/v1/storage", &result)
}
func (c *Client) Collect(ctx context.Context) (GarbageCollection, error) {
	var result GarbageCollection
	return result, c.do(ctx, http.MethodPost, "/v1/garbage-collections", &result)
}
func (c *Client) do(ctx context.Context, method, path string, output any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://registry-maintenance"+path, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("contact registry maintenance agent: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("registry maintenance agent returned status %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}
