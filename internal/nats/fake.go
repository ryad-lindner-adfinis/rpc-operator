/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package nats

import (
	"context"
	"sync"
)

// FakeManager is an in-memory StreamManager for tests. It records streams and KV
// buckets (keyed natsURL|name) and supports failure injection.
type FakeManager struct {
	mu         sync.Mutex
	Streams    map[string]string  // "natsURL|stream" -> subject
	Buckets    map[string]struct{} // "natsURL|bucket"
	EnsureFn   func(stream, subject string) error
	EnsureKVFn func(bucket string) error
}

var _ StreamManager = (*FakeManager)(nil)

func NewFakeManager() *FakeManager {
	return &FakeManager{
		Streams: map[string]string{},
		Buckets: map[string]struct{}{},
	}
}

func (f *FakeManager) key(url, stream string) string { return url + "|" + stream }

func (f *FakeManager) EnsureStream(_ context.Context, natsURL, stream, subject string, _ Retention) error {
	if f.EnsureFn != nil {
		if err := f.EnsureFn(stream, subject); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Streams[f.key(natsURL, stream)] = subject
	return nil
}

func (f *FakeManager) DeleteStream(_ context.Context, natsURL, stream string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Streams, f.key(natsURL, stream))
	return nil
}

// Has reports whether a stream is currently present (test helper).
func (f *FakeManager) Has(natsURL, stream string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Streams[f.key(natsURL, stream)]
	return ok
}

func (f *FakeManager) EnsureKV(_ context.Context, natsURL, bucket string, _ KVConfig) error {
	if f.EnsureKVFn != nil {
		if err := f.EnsureKVFn(bucket); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Buckets[f.key(natsURL, bucket)] = struct{}{}
	return nil
}

func (f *FakeManager) DeleteKV(_ context.Context, natsURL, bucket string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Buckets, f.key(natsURL, bucket))
	return nil
}

// HasKV reports whether a KV bucket is currently present (test helper).
func (f *FakeManager) HasKV(natsURL, bucket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Buckets[f.key(natsURL, bucket)]
	return ok
}
