/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	defaultImage    = "docker.redpanda.com/redpandadata/connect:4"
	configMountPath = "/etc/rpc"
	configFileName  = "pipeline.yaml"
	httpPort        = 4195
	rpcUID          = int64(10001)
)

func buildPodSpec(cmName, image string, envVars []corev1.EnvVar) corev1.PodSpec {
	if image == "" {
		image = defaultImage
	}
	return corev1.PodSpec{
		// OnFailure (not Always): a finite RPC pipeline (e.g. generate with
		// count=N) exits 0 when its work is done. Always would restart it
		// indefinitely, replaying the same input. OnFailure still restarts on
		// crashes, which is what long-running pipelines need.
		RestartPolicy:                 corev1.RestartPolicyOnFailure,
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
			Args:  []string{"run", configMountPath + "/" + configFileName},
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			Env: envVars,
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
						Key:  configFileName,
						Path: configFileName,
					}},
				},
			},
		}},
	}
}
