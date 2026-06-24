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

// FakeClient is an in-memory Client for tests. It records streams and cache
// resources per pod base URL, and supports pod restart simulation via DropPod.
type FakeClient struct {
	mu      sync.Mutex
	streams map[string]map[string]string // podBaseURL -> streamID -> configYAML
	caches  map[string]map[string]string // podBaseURL -> label -> configYAML
	// EnsureCount counts every EnsureStream call (test helper) so tests can assert
	// the reconciler does not re-deploy an unchanged stream on a periodic resync.
	EnsureCount int
	// EnsureErr, when non-nil, is returned by EnsureStream instead of recording.
	EnsureErr error
	// DropNextEnsure, when true, makes the next EnsureStream return nil WITHOUT
	// recording the stream (models a 2xx PUT that the instance does not load). One-shot.
	DropNextEnsure bool
	// EnsureCacheErr, when non-nil, is returned by EnsureCacheResource instead of recording.
	EnsureCacheErr error
	// GetErr, when non-nil, is returned by GetStreamStatus instead of a status.
	GetErr error
	// inactive marks stream ids that are held but report Active:false.
	inactive map[string]bool
}

var _ Client = (*FakeClient)(nil)

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		streams:  map[string]map[string]string{},
		caches:   map[string]map[string]string{},
		inactive: map[string]bool{},
	}
}

func (f *FakeClient) EnsureStream(_ context.Context, podBaseURL, streamID, configYAML string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EnsureCount++
	if f.EnsureErr != nil {
		return f.EnsureErr
	}
	if f.DropNextEnsure {
		f.DropNextEnsure = false
		return nil
	}
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

// DropPod removes all streams and cache resources on a pod, simulating a restart.
func (f *FakeClient) DropPod(podBaseURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.streams, podBaseURL)
	delete(f.caches, podBaseURL)
}

func (f *FakeClient) EnsureCacheResource(_ context.Context, podBaseURL, label, configYAML string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.EnsureCacheErr != nil {
		return f.EnsureCacheErr
	}
	if f.caches[podBaseURL] == nil {
		f.caches[podBaseURL] = map[string]string{}
	}
	f.caches[podBaseURL][label] = configYAML
	return nil
}

func (f *FakeClient) DeleteCacheResource(_ context.Context, podBaseURL, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.caches[podBaseURL], label)
	return nil
}

// HasCacheResource reports whether a label exists on a pod (test helper).
func (f *FakeClient) HasCacheResource(podBaseURL, label string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.caches[podBaseURL][label]
	return ok
}

// CacheResourceBody returns the stored config for a label (test helper).
func (f *FakeClient) CacheResourceBody(podBaseURL, label string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.caches[podBaseURL][label]
}

// StreamBody returns the configYAML stored for a stream (test helper).
// Returns "" if the stream or pod URL is not found.
func (f *FakeClient) StreamBody(podBaseURL, streamID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[podBaseURL][streamID]
}

// GetStreamStatus reports a held stream as Active:true unless marked inactive via
// SetStreamActive. A stream not held on the pod returns ErrStreamNotFound. GetErr,
// if set, is returned instead.
func (f *FakeClient) GetStreamStatus(_ context.Context, podBaseURL, streamID string) (StreamStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.GetErr != nil {
		return StreamStatus{}, f.GetErr
	}
	if _, ok := f.streams[podBaseURL][streamID]; !ok {
		return StreamStatus{}, ErrStreamNotFound
	}
	if f.inactive[streamID] {
		return StreamStatus{Active: false}, nil
	}
	return StreamStatus{Active: true, Uptime: 1}, nil // uptime arbitrary; consumers/tests assert Active only
}

// SetStreamActive marks whether a stream id reports Active in GetStreamStatus (test helper).
func (f *FakeClient) SetStreamActive(streamID string, active bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inactive[streamID] = !active
}
