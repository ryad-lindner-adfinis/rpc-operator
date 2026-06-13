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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	// CacheResources are project-global Redpanda Connect cache resources the
	// operator pushes to every instance of the project's cluster. Every pipeline
	// in the project can reference them by label (e.g. processors.cache.resource).
	// Each entry is exactly one of natsKV (managed) or config (custom). F51.
	// +optional
	// +listType=map
	// +listMapKey=name
	CacheResources []ProjectCacheResource `json:"cacheResources,omitempty"`
}

// ProjectClusterSpec passes through sizing to the managed PipelineCluster.
//
// Image is intentionally NOT exposed here in v1: the Redpanda Connect image is
// the runtime engine, not a per-project concern. The managed PipelineCluster
// picks up the operator-wide default from its own +kubebuilder:default. A
// future per-project image override (if a use case appears) would be additive.
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

// ProjectCacheResource is one project-global cache resource. Exactly one of
// NatsKV (managed: operator provisions the KV bucket and renders the config) or
// Config (custom: a native RPC cache config block pushed verbatim) must be set.
type ProjectCacheResource struct {
	// Name is the RPC resource label pipelines reference. Unique per project.
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// NatsKV configures a managed NATS KV cache. The operator creates the bucket
	// rpc-<project>-<name> on the project NATS and renders the nats_kv config.
	// +optional
	NatsKV *ProjectNATSKVCache `json:"natsKV,omitempty"`

	// Config is a native RPC cache config block (e.g. {redis: {url: ...}}) pushed
	// verbatim. The operator provisions nothing for custom resources.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Config runtime.RawExtension `json:"config,omitempty"`
}

// ProjectNATSKVCache sizes the managed KV bucket. All fields optional.
type ProjectNATSKVCache struct {
	// TTL expires each key after this duration. Unset = no expiry.
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// History is the number of historical values kept per key. Default 1.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	// +optional
	History *int64 `json:"history,omitempty"`

	// MaxBytes caps the bucket's total on-disk size. Unset = unlimited.
	// +optional
	MaxBytes *resource.Quantity `json:"maxBytes,omitempty"`
}

// ProjectCacheResourceStatus reports one cache resource's provisioning state.
type ProjectCacheResourceStatus struct {
	// Name matches spec.cacheResources[].name and is the join key.
	Name string `json:"name"`

	// Bucket is the managed KV bucket name (natsKV only): rpc-<project>-<name>.
	// +optional
	Bucket string `json:"bucket,omitempty"`

	// Phase is Ready or Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions report per-resource problems (e.g. BucketFailed, PushFailed).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

	// CacheResources mirrors per-cache-resource status. F51.
	// +optional
	CacheResources []ProjectCacheResourceStatus `json:"cacheResources,omitempty"`

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

// ProjectRouteStatus captures per-route runtime state. Populated by Phase 2 once
// the operator manages the route's JetStream stream and consumer wiring; left
// empty in Phase 1.
type ProjectRouteStatus struct {
	// Name matches the route's spec.routes[].name and is the join key.
	Name string `json:"name"`

	// Subject is the NATS subject backing this route's stream
	// (rpc.<project>.<route>). Empty until the operator has created the stream.
	Subject string `json:"subject,omitempty"`

	// Stream is the JetStream stream name backing this route
	// (rpc-<project>-<route>).
	Stream string `json:"stream,omitempty"`

	// Phase is the route's lifecycle state. Phase 2 defines the value set.
	Phase string `json:"phase,omitempty"`

	// Conditions report per-route problems (e.g., stream creation failures).
	// +listType=map
	// +listMapKey=type
	// +optional
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
