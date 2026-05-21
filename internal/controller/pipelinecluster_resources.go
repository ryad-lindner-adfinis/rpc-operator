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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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
// Namespace is intentionally unset; the reconciler keys the object before applying.
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

// buildClusterStatefulSet renders the StatefulSet of streams-mode Connect
// instances. Stable pod identities (StatefulSet) enable pod-addressable stream
// placement in Phase 2. RestartPolicy is the StatefulSet default (Always):
// these are long-running servers, unlike the finite single-pod Pipeline model.
// Namespace is intentionally unset; the reconciler keys the object before applying.
func buildClusterStatefulSet(
	clusterName, image string,
	replicas int32,
	resources corev1.ResourceRequirements,
	cmName, svcName string,
) *appsv1.StatefulSet {
	if image == "" {
		image = defaultImage
	}
	labels := map[string]string{clusterLabelKey: clusterName}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To(replicas),
			ServiceName: svcName,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr.To[int64](30),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(rpcUID),
						FSGroup:      ptr.To(rpcUID),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Name:  "connect",
						Image: image,
						Args:  []string{"-c", configMountPath + "/" + clusterConfigFile, "streams"},
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: httpPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path: "/ping",
								Port: intstr.FromString("http"),
							}},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path: "/ready",
								Port: intstr.FromString("http"),
							}},
							InitialDelaySeconds: 2,
							PeriodSeconds:       5,
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							ReadOnlyRootFilesystem:   ptr.To(true),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Resources: resources,
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "cfg",
							MountPath: configMountPath,
							ReadOnly:  true,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "cfg",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
								Items: []corev1.KeyToPath{{
									Key:  clusterConfigFile,
									Path: clusterConfigFile,
								}},
							},
						},
					}},
				},
			},
		},
	}
}
