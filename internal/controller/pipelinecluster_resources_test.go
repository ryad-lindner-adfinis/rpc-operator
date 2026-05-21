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
