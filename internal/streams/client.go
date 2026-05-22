/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package streams talks to the per-pod Redpanda Connect streams HTTP API
// (POST/PUT/DELETE/GET /streams/{id}) used by F47 PipelineClusters.
package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client manages streams on a single Redpanda Connect instance addressed by its
// base URL (e.g. http://etl-small-1.etl-small.ns.svc:4195).
type Client interface {
	// EnsureStream upserts a stream with the given id and config (PUT is idempotent).
	EnsureStream(ctx context.Context, podBaseURL, streamID, configYAML string) error
	// DeleteStream removes a stream; a 404 (already gone) is treated as success.
	DeleteStream(ctx context.Context, podBaseURL, streamID string) error
	// ListStreams returns the set of stream ids currently running on the instance.
	ListStreams(ctx context.Context, podBaseURL string) (map[string]struct{}, error)
}

// HTTPClient is the production Client over HTTP.
type HTTPClient struct {
	HTTP *http.Client
}

var _ Client = (*HTTPClient)(nil)

// NewHTTPClient returns an HTTPClient with a sane request timeout.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// EnsureStream upserts a stream. The Redpanda Connect streams API uses POST to
// create and PUT to update; a PUT on a stream that does not exist yet returns
// 404. So we PUT first (the common case once a stream exists) and fall back to
// POST when the stream is absent.
func (c *HTTPClient) EnsureStream(ctx context.Context, podBaseURL, streamID, configYAML string) error {
	status, body, err := c.streamReq(ctx, http.MethodPut, podBaseURL, streamID, configYAML)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		status, body, err = c.streamReq(ctx, http.MethodPost, podBaseURL, streamID, configYAML)
		if err != nil {
			return err
		}
	}
	if status >= 300 {
		return fmt.Errorf("ensure stream %s: status %d: %s", streamID, status, body)
	}
	return nil
}

// streamReq sends one config-bearing request to /streams/{id}, drains the
// response body, and returns the status code and body text.
func (c *HTTPClient) streamReq(ctx context.Context, method, podBaseURL, streamID, configYAML string) (int, string, error) {
	url := fmt.Sprintf("%s/streams/%s", strings.TrimRight(podBaseURL, "/"), streamID)
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(configYAML))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("%s stream %s: %w", method, streamID, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func (c *HTTPClient) DeleteStream(ctx context.Context, podBaseURL, streamID string) error {
	url := fmt.Sprintf("%s/streams/%s", strings.TrimRight(podBaseURL, "/"), streamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE stream %s: %w", streamID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE stream %s: status %d: %s", streamID, resp.StatusCode, string(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *HTTPClient) ListStreams(ctx context.Context, podBaseURL string) (map[string]struct{}, error) {
	url := fmt.Sprintf("%s/streams", strings.TrimRight(podBaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET streams: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET streams: status %d: %s", resp.StatusCode, string(body))
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode streams: %w", err)
	}
	out := make(map[string]struct{}, len(raw))
	for id := range raw {
		out[id] = struct{}{}
	}
	return out, nil
}
