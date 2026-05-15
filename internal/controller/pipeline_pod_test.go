/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func TestDerivePhase(t *testing.T) {
	cases := []struct {
		in   corev1.PodPhase
		want rpcv1alpha1.PipelinePhase
	}{
		{corev1.PodPending, rpcv1alpha1.PhasePending},
		{corev1.PodRunning, rpcv1alpha1.PhaseRunning},
		{corev1.PodFailed, rpcv1alpha1.PhaseFailed},
		{corev1.PodSucceeded, rpcv1alpha1.PhaseStopped},
		{corev1.PodUnknown, rpcv1alpha1.PhasePending},
		{corev1.PodPhase(""), rpcv1alpha1.PhasePending},
	}
	for _, tc := range cases {
		pod := &corev1.Pod{Status: corev1.PodStatus{Phase: tc.in}}
		if got := derivePhase(pod); got != tc.want {
			t.Errorf("derivePhase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildPodSpec_Defaults(t *testing.T) {
	spec := buildPodSpec("hello-config", "", nil)
	if spec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Errorf("expected RestartPolicy=OnFailure, got %q", spec.RestartPolicy)
	}
	if len(spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(spec.Containers))
	}
	c := spec.Containers[0]
	if c.Image != defaultImage {
		t.Errorf("expected default image %q, got %q", defaultImage, c.Image)
	}
	if c.Args[0] != "run" || c.Args[1] != "/etc/rpc/pipeline.yaml" {
		t.Errorf("unexpected args: %v", c.Args)
	}
	if c.LivenessProbe == nil || c.LivenessProbe.HTTPGet.Path != "/ping" {
		t.Errorf("liveness probe missing or wrong path")
	}
	if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet.Path != "/ready" {
		t.Errorf("readiness probe missing or wrong path")
	}
	if spec.SecurityContext == nil || spec.SecurityContext.RunAsUser == nil ||
		*spec.SecurityContext.RunAsUser != rpcUID {
		t.Errorf("expected RunAsUser=%d", rpcUID)
	}
	if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil ||
		!*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected ReadOnlyRootFilesystem=true")
	}
	if len(spec.Volumes) != 1 || spec.Volumes[0].ConfigMap == nil ||
		spec.Volumes[0].ConfigMap.Name != "hello-config" {
		t.Errorf("expected configmap volume 'hello-config'")
	}
}

func TestBuildPodSpec_CustomImage(t *testing.T) {
	spec := buildPodSpec("cm", "ghcr.io/redpanda-data/connect:4.36.1", nil)
	if spec.Containers[0].Image != "ghcr.io/redpanda-data/connect:4.36.1" {
		t.Errorf("custom image not propagated: %v", spec.Containers[0].Image)
	}
}

func TestBuildPodSpec_SecretRefs(t *testing.T) {
	envVars := []corev1.EnvVar{{
		Name: "MY_SECRET",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
				Key:                  "password",
			},
		},
	}}
	spec := buildPodSpec("cm", "", envVars)
	if len(spec.Containers[0].Env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(spec.Containers[0].Env))
	}
	env := spec.Containers[0].Env[0]
	if env.Name != "MY_SECRET" {
		t.Errorf("expected env name MY_SECRET, got %s", env.Name)
	}
	if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
		t.Fatal("expected SecretKeyRef, got nil")
	}
	if env.ValueFrom.SecretKeyRef.Name != "my-secret" || env.ValueFrom.SecretKeyRef.Key != "password" {
		t.Errorf("unexpected SecretKeyRef: %+v", env.ValueFrom.SecretKeyRef)
	}
}

func TestBuildPodSpec_NoEnvWhenNilRefs(t *testing.T) {
	spec := buildPodSpec("cm", "", nil)
	if len(spec.Containers[0].Env) != 0 {
		t.Errorf("expected no env vars for nil secretRefs, got %d", len(spec.Containers[0].Env))
	}
}
