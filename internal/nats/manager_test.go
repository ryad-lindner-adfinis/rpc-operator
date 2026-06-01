package nats

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFakeManager_EnsureAndDelete(t *testing.T) {
	f := NewFakeManager()
	ctx := context.Background()
	url := "nats://orders-nats.ns.svc:4222"

	if err := f.EnsureStream(ctx, url, "rpc-orders-a", "rpc.orders.a", Retention{MaxAge: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if !f.Has(url, "rpc-orders-a") {
		t.Fatal("stream should exist after ensure")
	}
	if err := f.DeleteStream(ctx, url, "rpc-orders-a"); err != nil {
		t.Fatal(err)
	}
	if f.Has(url, "rpc-orders-a") {
		t.Fatal("stream should be gone after delete")
	}
	if err := f.DeleteStream(ctx, url, "ghost"); err != nil {
		t.Fatalf("delete missing should succeed: %v", err)
	}
}

func TestFakeManager_EnsureFnFailureNotRecorded(t *testing.T) {
	f := NewFakeManager()
	f.EnsureFn = func(stream, subject string) error {
		return errors.New("boom")
	}
	url := "nats://orders-nats.ns.svc:4222"
	if err := f.EnsureStream(context.Background(), url, "rpc-orders-a", "rpc.orders.a", Retention{}); err == nil {
		t.Fatal("expected injected error")
	}
	if f.Has(url, "rpc-orders-a") {
		t.Fatal("stream must NOT be recorded when EnsureFn fails")
	}
}
