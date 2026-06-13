package nats

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errKVBoom = errors.New("kv boom")

func TestFakeManager_EnsureAndDeleteKV(t *testing.T) {
	f := NewFakeManager()
	ctx := context.Background()
	url := "nats://orders-nats.ns.svc:4222"

	if err := f.EnsureKV(ctx, url, "rpc-orders-shared", KVConfig{TTL: time.Hour, History: 3}); err != nil {
		t.Fatal(err)
	}
	if !f.HasKV(url, "rpc-orders-shared") {
		t.Fatal("bucket should exist after EnsureKV")
	}
	if err := f.DeleteKV(ctx, url, "rpc-orders-shared"); err != nil {
		t.Fatal(err)
	}
	if f.HasKV(url, "rpc-orders-shared") {
		t.Fatal("bucket should be gone after DeleteKV")
	}
	if err := f.DeleteKV(ctx, url, "ghost"); err != nil {
		t.Fatalf("delete missing bucket should succeed: %v", err)
	}
}

func TestFakeManager_EnsureKVFailureNotRecorded(t *testing.T) {
	f := NewFakeManager()
	f.EnsureKVFn = func(bucket string) error { return errKVBoom }
	url := "nats://orders-nats.ns.svc:4222"
	if err := f.EnsureKV(context.Background(), url, "rpc-orders-shared", KVConfig{}); err == nil {
		t.Fatal("expected injected error")
	}
	if f.HasKV(url, "rpc-orders-shared") {
		t.Fatal("bucket must NOT be recorded when EnsureKVFn fails")
	}
}
