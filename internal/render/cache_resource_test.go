package render

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func TestBuildCacheResourceConfig_NatsKV(t *testing.T) {
	cr := rpcv1alpha1.ProjectCacheResource{Name: "shared", NatsKV: &rpcv1alpha1.ProjectNATSKVCache{}}
	out, err := BuildCacheResourceConfig(cr, "nats://orders-nats.ns.svc:4222", "rpc-orders-shared")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"nats_kv:", "rpc-orders-shared", "nats://orders-nats.ns.svc:4222"} {
		if !strings.Contains(out, want) {
			t.Fatalf("config %q missing %q", out, want)
		}
	}
}

func TestBuildCacheResourceConfig_Custom(t *testing.T) {
	cr := rpcv1alpha1.ProjectCacheResource{
		Name:   "r",
		Config: runtime.RawExtension{Raw: []byte(`{"redis":{"url":"redis://h:6379"}}`)},
	}
	out, err := BuildCacheResourceConfig(cr, "ignored", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "redis:") || !strings.Contains(out, "redis://h:6379") {
		t.Fatalf("custom config not rendered: %q", out)
	}
}

func TestBuildCacheResourceConfig_CustomInvalidJSON(t *testing.T) {
	cr := rpcv1alpha1.ProjectCacheResource{Name: "r", Config: runtime.RawExtension{Raw: []byte(`{bad`)}}
	if _, err := BuildCacheResourceConfig(cr, "x", "y"); err == nil {
		t.Fatal("expected error for invalid custom config JSON")
	}
}
