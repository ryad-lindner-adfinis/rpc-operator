/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const clusterConfigFile = "connect.yaml"

// clusterLabelKey marks pods/resources as belonging to a named PipelineCluster.
const clusterLabelKey = "rpc.operator.io/cluster"

// clusterConfigYAML returns the Redpanda Connect main config loaded by every
// streams-mode instance in a PipelineCluster. It enables the HTTP server (for
// the streams API + health probes on httpPort) and sets the logger format.
func clusterConfigYAML(jsonLogging bool) string {
	format := "logfmt"
	if jsonLogging {
		format = "json"
	}
	return fmt.Sprintf(`http:
  address: 0.0.0.0:%d
  enabled: true
logger:
  level: INFO
  format: %s
  add_timestamp: true
`, httpPort, format)
}

// buildClusterService returns a headless Service fronting the cluster's pods.
// Headless (ClusterIP: None) because the streams API is pod-local — Phase 2
// addresses individual pods by their stable StatefulSet DNS names.
func buildClusterService(clusterName, svcName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: svcName},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  map[string]string{clusterLabelKey: clusterName},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       httpPort,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}
