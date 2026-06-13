/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package nats manages the per-route JetStream streams a PipelineProject owns.
// Each route maps to exactly one stream (one subject), created/updated/deleted
// from the project's spec.routes[]. The interface is mocked in tests; the real
// implementation dials the project's in-cluster NATS over its headless Service.
package nats

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Retention is the per-stream limit set the operator applies. Zero-value fields
// mean "no limit" (JetStream default).
type Retention struct {
	MaxAge   time.Duration
	MaxBytes int64
	MaxMsgs  int64
}

// KVConfig sizes a managed NATS KV bucket. Zero-value fields mean "default":
// History 0 → 1, TTL/MaxBytes 0 → unlimited.
type KVConfig struct {
	TTL      time.Duration
	History  uint8
	MaxBytes int64
}

// StreamManager creates/updates/deletes JetStream streams and KV buckets for
// project routes and cache resources. natsURL is the project's client URL
// (projectroute.NATSURL). Implementations must be safe for repeated calls.
type StreamManager interface {
	// EnsureStream upserts a single-subject stream. Idempotent.
	EnsureStream(ctx context.Context, natsURL, stream, subject string, r Retention) error
	// DeleteStream removes a stream; a missing stream is treated as success.
	DeleteStream(ctx context.Context, natsURL, stream string) error
	// EnsureKV upserts a NATS KV bucket. Idempotent.
	EnsureKV(ctx context.Context, natsURL, bucket string, cfg KVConfig) error
	// DeleteKV removes a KV bucket; a missing bucket is treated as success.
	DeleteKV(ctx context.Context, natsURL, bucket string) error
}

// JSManager is the production StreamManager over nats.go/JetStream. It dials per
// call and closes the connection; project reconciles are infrequent so a pooled
// connection is not worth the lifecycle complexity in v1.
type JSManager struct {
	// DialTimeout caps connection establishment. Default 5s if zero.
	DialTimeout time.Duration
}

var _ StreamManager = (*JSManager)(nil)

func (m *JSManager) connect(natsURL string) (*nats.Conn, jetstream.JetStream, error) {
	to := m.DialTimeout
	if to == 0 {
		to = 5 * time.Second
	}
	nc, err := nats.Connect(natsURL, nats.Timeout(to))
	if err != nil {
		return nil, nil, err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	return nc, js, nil
}

func (m *JSManager) EnsureStream(ctx context.Context, natsURL, stream, subject string, r Retention) error {
	nc, js, err := m.connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	cfg := jetstream.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    r.MaxAge,
		MaxBytes:  r.MaxBytes,
		MaxMsgs:   r.MaxMsgs,
	}
	_, err = js.CreateOrUpdateStream(ctx, cfg)
	return err
}

func (m *JSManager) DeleteStream(ctx context.Context, natsURL, stream string) error {
	nc, js, err := m.connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	err = js.DeleteStream(ctx, stream)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return nil
	}
	return err
}

func (m *JSManager) EnsureKV(ctx context.Context, natsURL, bucket string, cfg KVConfig) error {
	nc, js, err := m.connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	history := cfg.History
	if history == 0 {
		history = 1
	}
	_, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   bucket,
		TTL:      cfg.TTL,
		History:  history,
		MaxBytes: cfg.MaxBytes,
		Storage:  jetstream.FileStorage,
	})
	return err
}

func (m *JSManager) DeleteKV(ctx context.Context, natsURL, bucket string) error {
	nc, js, err := m.connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	err = js.DeleteKeyValue(ctx, bucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil
	}
	return err
}
