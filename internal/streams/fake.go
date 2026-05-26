/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package streams

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client for tests. It records streams per pod base
// URL so specs can assert placement and simulate a pod restart (DropPod).
type FakeClient struct {
	mu      sync.Mutex
	streams map[string]map[string]string // podBaseURL -> streamID -> configYAML
}

var _ Client = (*FakeClient)(nil)

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{streams: map[string]map[string]string{}}
}

func (f *FakeClient) EnsureStream(_ context.Context, podBaseURL, streamID, configYAML string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams[podBaseURL] == nil {
		f.streams[podBaseURL] = map[string]string{}
	}
	f.streams[podBaseURL][streamID] = configYAML
	return nil
}

func (f *FakeClient) DeleteStream(_ context.Context, podBaseURL, streamID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.streams[podBaseURL], streamID)
	return nil
}

func (f *FakeClient) ListStreams(_ context.Context, podBaseURL string) (map[string]struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]struct{}{}
	for id := range f.streams[podBaseURL] {
		out[id] = struct{}{}
	}
	return out, nil
}

// Has reports whether a stream id exists on the given pod (test helper).
func (f *FakeClient) Has(podBaseURL, streamID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.streams[podBaseURL][streamID]
	return ok
}

// DropPod removes all streams on a pod, simulating a pod restart (test helper).
func (f *FakeClient) DropPod(podBaseURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.streams, podBaseURL)
}

// StreamBody returns the configYAML stored for a stream (test helper).
// Returns "" if the stream or pod URL is not found.
func (f *FakeClient) StreamBody(podBaseURL, streamID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[podBaseURL][streamID]
}
