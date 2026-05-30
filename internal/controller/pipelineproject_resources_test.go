/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func TestProjectChildNames(t *testing.T) {
	if got := projectChildClusterName("orders"); got != "orders-cluster" {
		t.Fatalf("projectChildClusterName: got %q want orders-cluster", got)
	}
	if got := projectChildNATSName("orders"); got != "orders-nats" {
		t.Fatalf("projectChildNATSName: got %q want orders-nats", got)
	}
}

func TestBuildProjectCluster_DefaultsToSingleInstance(t *testing.T) {
	pp := &rpcv1alpha1.PipelineProject{}
	pp.Name = "orders"

	pc := buildProjectCluster(pp)

	if pc.Name != "orders-cluster" {
		t.Errorf("Name: got %q want orders-cluster", pc.Name)
	}
	if pc.Spec.Replicas != 1 {
		t.Errorf("Replicas: got %d want 1", pc.Spec.Replicas)
	}
	if !pc.Spec.JSONLogging {
		t.Errorf("JSONLogging: should default to true (F47 needs it for per-stream filter)")
	}
}

func TestBuildProjectCluster_HonorsSpec(t *testing.T) {
	pp := &rpcv1alpha1.PipelineProject{
		Spec: rpcv1alpha1.PipelineProjectSpec{
			Cluster: &rpcv1alpha1.ProjectClusterSpec{
				Instances: ptr.To[int32](3),
			},
		},
	}
	pp.Name = "orders"

	pc := buildProjectCluster(pp)

	if pc.Spec.Replicas != 3 {
		t.Errorf("Replicas: got %d want 3", pc.Spec.Replicas)
	}
}

func TestNATSServerConfig_SingleReplicaOmitsCluster(t *testing.T) {
	cfg := natsServerConfig("orders", 1)

	if !contains(cfg, "jetstream") {
		t.Errorf("config should enable JetStream; got: %s", cfg)
	}
	if contains(cfg, "cluster {") {
		t.Errorf("single-replica config must NOT include cluster routes; got: %s", cfg)
	}
}

func TestNATSServerConfig_MultiReplicaIncludesCluster(t *testing.T) {
	cfg := natsServerConfig("orders", 3)

	if !contains(cfg, "cluster {") {
		t.Errorf("multi-replica config must include cluster routes; got: %s", cfg)
	}
	if !contains(cfg, "nats-route://orders-nats-0.orders-nats:6222") {
		t.Errorf("multi-replica config must include peer-0 route; got: %s", cfg)
	}
}

func TestBuildProjectNATSStatefulSet_PVCStorage(t *testing.T) {
	storage := resource.MustParse("5Gi")
	ss := buildProjectNATSStatefulSet("orders", "", 1, storage)

	if len(ss.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("expected one VolumeClaimTemplate, got %d", len(ss.Spec.VolumeClaimTemplates))
	}
	got := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
	if got.String() != "5Gi" {
		t.Errorf("PVC storage: got %s want 5Gi", got.String())
	}
}

func TestBuildProjectNATSStatefulSet_DefaultStorage(t *testing.T) {
	ss := buildProjectNATSStatefulSet("orders", "", 1, resource.Quantity{})
	got := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
	if got.String() != "10Gi" {
		t.Errorf("default storage: got %s want 10Gi", got.String())
	}
}

func TestBuildProjectNATSStatefulSet_DefaultImage(t *testing.T) {
	ss := buildProjectNATSStatefulSet("orders", "", 1, resource.MustParse("1Gi"))
	if ss.Spec.Template.Spec.Containers[0].Image != natsImageDefault {
		t.Errorf("default image: got %q want %q", ss.Spec.Template.Spec.Containers[0].Image, natsImageDefault)
	}
}

func TestBuildProjectNATSStatefulSet_OverrideImage(t *testing.T) {
	ss := buildProjectNATSStatefulSet("orders", "myregistry/nats:2.10.20", 1, resource.MustParse("1Gi"))
	if ss.Spec.Template.Spec.Containers[0].Image != "myregistry/nats:2.10.20" {
		t.Errorf("override image not honored: got %q", ss.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestBuildProjectNATSService_IsHeadless(t *testing.T) {
	svc := buildProjectNATSService("orders")
	if svc.Spec.ClusterIP != "None" {
		t.Errorf("ClusterIP: got %q want None (headless required for JS peer DNS)", svc.Spec.ClusterIP)
	}
}

// contains is a small test helper to avoid pulling strings into every test.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
