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
	svc := buildClusterService("etl-small", "etl-small")
	if svc.Name != "etl-small" {
		t.Errorf("expected service name etl-small, got %q", svc.Name)
	}
	if svc.Spec.ClusterIP != "None" {
		t.Errorf("expected headless service (ClusterIP None), got %q", svc.Spec.ClusterIP)
	}
	if svc.Spec.Selector[clusterLabelKey] != "etl-small" {
		t.Errorf("expected selector %s=etl-small, got %v", clusterLabelKey, svc.Spec.Selector)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != httpPort {
		t.Errorf("expected single port %d, got %v", httpPort, svc.Spec.Ports)
	}
}

func TestBuildClusterStatefulSet(t *testing.T) {
	ss := buildClusterStatefulSet("etl-small", "", 3, corev1.ResourceRequirements{}, "etl-small-config", "etl-small")

	if ss.Name != "etl-small" {
		t.Errorf("expected name etl-small, got %q", ss.Name)
	}
	if ss.Spec.Replicas == nil || *ss.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %v", ss.Spec.Replicas)
	}
	if ss.Spec.ServiceName != "etl-small" {
		t.Errorf("expected serviceName etl-small, got %q", ss.Spec.ServiceName)
	}
	if ss.Spec.Selector.MatchLabels[clusterLabelKey] != "etl-small" {
		t.Errorf("expected selector %s=etl-small, got %v", clusterLabelKey, ss.Spec.Selector.MatchLabels)
	}
	if ss.Spec.Template.Labels[clusterLabelKey] != "etl-small" {
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
	if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet.Path != "/ready" {
		t.Errorf("expected readiness probe on /ready")
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
