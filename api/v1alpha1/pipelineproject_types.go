/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineProjectSpec defines the desired state of a PipelineProject — a
// user-facing grouping of Pipelines backed by a system-managed PipelineCluster
// and NATS JetStream StatefulSet. Phase 1 only provisions the runtime; routes
// are accepted in the spec but not yet acted on.
type PipelineProjectSpec struct {
	// Description is an optional human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// Cluster sizes the underlying PipelineCluster. All fields optional.
	// +optional
	Cluster *ProjectClusterSpec `json:"cluster,omitempty"`

	// NATS sizes and configures the per-Project JetStream StatefulSet.
	// +optional
	NATS *ProjectNATSSpec `json:"nats,omitempty"`

	// Routes is the declarative routing table wiring Pipelines via NATS subjects.
	// Phase 1 stores the field but does not process it. Phase 2 implements
	// stream provisioning and Pipeline I/O rewriting from this table.
	// +optional
	Routes []ProjectRoute `json:"routes,omitempty"`
}

// ProjectClusterSpec passes through sizing to the managed PipelineCluster.
type ProjectClusterSpec struct {
	// Instances is the number of streams-mode Connect instances.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Instances *int32 `json:"instances,omitempty"`

	// Resources sets CPU/memory requests and limits per instance container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ProjectNATSSpec configures the per-Project NATS JetStream StatefulSet.
type ProjectNATSSpec struct {
	// Replicas is the number of NATS server replicas. v1 supports 1; ≥3 is
	// accepted as config but only smoke-tested.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Storage is the per-replica JetStream file storage PVC size.
	// +optional
	Storage *resource.Quantity `json:"storage,omitempty"`

	// Retention sets the default JetStream stream retention applied to every
	// route's stream. Per-route overrides are honored in Phase 2.
	// +optional
	Retention ProjectNATSRetention `json:"retention,omitempty"`

	// StorageReclaimPolicy controls PVC handling on Project deletion.
	// "Retain" (default) keeps PVCs; "Delete" removes them.
	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Enum=Retain;Delete
	// +optional
	StorageReclaimPolicy string `json:"storageReclaimPolicy,omitempty"`
}

// ProjectNATSRetention captures the JetStream stream limits the operator
// applies when creating route streams. Phase 1 stores defaults; Phase 2 uses
// them when calling the NATS JS API.
type ProjectNATSRetention struct {
	// MaxAge is the TTL after which messages are purged.
	// +optional
	MaxAge *metav1.Duration `json:"maxAge,omitempty"`

	// MaxBytes caps the stream's total on-disk size.
	// +optional
	MaxBytes *resource.Quantity `json:"maxBytes,omitempty"`

	// MaxMsgs caps the stream's message count.
	// +optional
	MaxMsgs *int64 `json:"maxMsgs,omitempty"`
}

// ProjectRoute is one declarative wire in the routing table. Phase 1 stores
// but does not act on routes; Phase 2 provisions NATS streams and rewrites
// affected Pipelines' I/O.
type ProjectRoute struct {
	// Name is unique within the Project. DNS-1123 label.
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// From is the producer Pipeline. Must reference a Pipeline in this
	// project with matching spec.projectRef once Phase 2 ships.
	From string `json:"from"`

	// To is the list of consumer Pipelines, optionally each with a Bloblang
	// predicate (Phase 2).
	// +kubebuilder:validation:MinItems=1
	To []ProjectRouteTarget `json:"to"`

	// Retention is an optional override of the Project's default retention
	// for this route's stream.
	// +optional
	Retention *ProjectNATSRetention `json:"retention,omitempty"`
}

// ProjectRouteTarget is one consumer of a route, optionally filtered.
type ProjectRouteTarget struct {
	// Pipeline is the target Pipeline name in the same namespace.
	Pipeline string `json:"pipeline"`

	// When is an optional Bloblang predicate evaluated consumer-side
	// (Phase 2). Empty = always deliver.
	// +optional
	When string `json:"when,omitempty"`
}

// PipelineProjectPhase reports the high-level lifecycle stage.
// +kubebuilder:validation:Enum=Provisioning;Ready;Degraded;Deleting
type PipelineProjectPhase string

const (
	ProjectPhaseProvisioning PipelineProjectPhase = "Provisioning"
	ProjectPhaseReady        PipelineProjectPhase = "Ready"
	ProjectPhaseDegraded     PipelineProjectPhase = "Degraded"
	ProjectPhaseDeleting     PipelineProjectPhase = "Deleting"
)

// PipelineProjectStatus describes the observed state.
type PipelineProjectStatus struct {
	// Phase is the high-level lifecycle stage.
	// +optional
	Phase PipelineProjectPhase `json:"phase,omitempty"`

	// Cluster reports the managed PipelineCluster's readiness.
	// +optional
	Cluster ProjectChildStatus `json:"cluster,omitempty"`

	// NATS reports the managed NATS StatefulSet's readiness.
	// +optional
	NATS ProjectChildStatus `json:"nats,omitempty"`

	// Routes mirrors per-route status (populated by Phase 2).
	// +optional
	Routes []ProjectRouteStatus `json:"routes,omitempty"`

	// ObservedGeneration is the .metadata.generation this status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ProjectChildStatus tracks readiness of a managed child resource.
type ProjectChildStatus struct {
	// Name is the child resource name.
	// +optional
	Name string `json:"name,omitempty"`

	// Ready is the number of ready replicas/instances.
	// +optional
	Ready int32 `json:"ready,omitempty"`

	// Total is the desired number of replicas/instances.
	// +optional
	Total int32 `json:"total,omitempty"`
}

// ProjectRouteStatus captures per-route runtime state (Phase 2).
type ProjectRouteStatus struct {
	Name       string             `json:"name"`
	Subject    string             `json:"subject,omitempty"`
	Stream     string             `json:"stream,omitempty"`
	Phase      string             `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ppr
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.status.cluster.name`
// +kubebuilder:printcolumn:name="ClusterReady",type=string,JSONPath=`.status.cluster.ready`
// +kubebuilder:printcolumn:name="NATSReady",type=string,JSONPath=`.status.nats.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PipelineProject is the Schema for the pipelineprojects API.
type PipelineProject struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PipelineProjectSpec `json:"spec"`

	// +optional
	Status PipelineProjectStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineProjectList contains a list of PipelineProject.
type PipelineProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PipelineProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PipelineProject{}, &PipelineProjectList{})
}
