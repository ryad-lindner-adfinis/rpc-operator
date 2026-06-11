/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const testClusterName = "etl-small"

func TestClusterConfigYAML_JSONLogging(t *testing.T) {
	cfg := clusterConfigYAML(true)
	if !strings.Contains(cfg, "format: json") {
		t.Errorf("expected json logger format, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "address: 0.0.0.0:4195") {
		t.Errorf("expected http address on 4195, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "enabled: true") {
		t.Errorf("expected http enabled, got:\n%s", cfg)
	}
}

func TestClusterConfigYAML_PlainLogging(t *testing.T) {
	cfg := clusterConfigYAML(false)
	if strings.Contains(cfg, "format: json") {
		t.Errorf("expected non-json format when jsonLogging=false, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "format: logfmt") {
		t.Errorf("expected logfmt format, got:\n%s", cfg)
	}
}

func TestBuildClusterService_Headless(t *testing.T) {
	svc := buildClusterService(testClusterName, testClusterName)
	if svc.Name != testClusterName {
		t.Errorf("expected service name etl-small, got %q", svc.Name)
	}
	if svc.Spec.ClusterIP != "None" {
		t.Errorf("expected headless service (ClusterIP None), got %q", svc.Spec.ClusterIP)
	}
	if svc.Spec.Selector[clusterLabelKey] != testClusterName {
		t.Errorf("expected selector %s=etl-small, got %v", clusterLabelKey, svc.Spec.Selector)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != httpPort {
		t.Errorf("expected single port %d, got %v", httpPort, svc.Spec.Ports)
	}
	if !svc.Spec.PublishNotReadyAddresses {
		t.Errorf("expected PublishNotReadyAddresses=true so the operator reaches NotReady pods by DNS")
	}
}

func TestBuildClusterStatefulSet(t *testing.T) {
	ss := buildClusterStatefulSet(testClusterName, "", 3, corev1.ResourceRequirements{}, "etl-small-config", testClusterName)

	if ss.Name != testClusterName {
		t.Errorf("expected name etl-small, got %q", ss.Name)
	}
	if ss.Spec.Replicas == nil || *ss.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %v", ss.Spec.Replicas)
	}
	if ss.Spec.ServiceName != testClusterName {
		t.Errorf("expected serviceName etl-small, got %q", ss.Spec.ServiceName)
	}
	if ss.Spec.Selector.MatchLabels[clusterLabelKey] != testClusterName {
		t.Errorf("expected selector %s=etl-small, got %v", clusterLabelKey, ss.Spec.Selector.MatchLabels)
	}
	if ss.Spec.Template.Labels[clusterLabelKey] != testClusterName {
		t.Errorf("expected pod label %s=etl-small, got %v", clusterLabelKey, ss.Spec.Template.Labels)
	}

	if len(ss.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(ss.Spec.Template.Spec.Containers))
	}
	c := ss.Spec.Template.Spec.Containers[0]
	if c.Image != defaultImage {
		t.Errorf("expected default image %q, got %q", defaultImage, c.Image)
	}
	wantArgs := []string{"-c", configMountPath + "/" + clusterConfigFile, "streams"}
	if len(c.Args) != 3 || c.Args[0] != wantArgs[0] || c.Args[1] != wantArgs[1] || c.Args[2] != wantArgs[2] {
		t.Errorf("expected args %v, got %v", wantArgs, c.Args)
	}
	if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet.Path != "/ping" {
		t.Errorf("expected readiness probe on /ping (API reachability, not per-stream connectivity)")
	}
	if c.LivenessProbe == nil || c.LivenessProbe.HTTPGet.Path != "/ping" {
		t.Errorf("expected liveness probe on /ping")
	}
	if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected ReadOnlyRootFilesystem=true")
	}
}

func TestBuildClusterStatefulSet_ImageOverride(t *testing.T) {
	ss := buildClusterStatefulSet("c", "custom/connect:1.2", 1, corev1.ResourceRequirements{}, "c-config", "c")
	if got := ss.Spec.Template.Spec.Containers[0].Image; got != "custom/connect:1.2" {
		t.Errorf("expected overridden image, got %q", got)
	}
}
