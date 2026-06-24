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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentSpec describes one Redpanda Connect component (input, processor, or output).
type ComponentSpec struct {
	// Type is the RPC component name, e.g. "generate", "kafka", "mapping", "stdout".
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Label is the Benthos component label used for metrics and tracing.
	// Required for processors; optional for input and output.
	// +optional
	Label string `json:"label,omitempty"`

	// Config is passed verbatim as the body of the component in the rendered YAML.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Config runtime.RawExtension `json:"config,omitempty"`
}

// SecretRef binds a Kubernetes Secret key to an environment variable name
// that is injected into the pipeline Pod container. Reference the variable
// in RPC YAML as ${ENV_VAR}.
type SecretRef struct {
	// EnvVar is the environment variable name exposed to the RPC process.
	// Must match [A-Za-z_][A-Za-z0-9_]*.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	EnvVar string `json:"envVar"`

	// SecretName is the name of the Kubernetes Secret in the same namespace.
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Key is the key within the Secret's data map.
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// PipelineSpec defines the desired state of Pipeline.
type PipelineSpec struct {
	// +optional
	Input ComponentSpec `json:"input,omitempty"`

	// +optional
	Processors []ComponentSpec `json:"processors,omitempty"`

	// +optional
	Output ComponentSpec `json:"output,omitempty"`

	// RawYAML holds a complete Redpanda Connect config in native YAML format.
	// When set, Input, Processors, and Output are ignored; no catalog validation
	// is performed. The HTTP server block is injected automatically if absent.
	// +optional
	RawYAML string `json:"rawYAML,omitempty"`

	// v0.1: only single-replica pipelines. Multi-replica is v0.4+.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// +kubebuilder:default="docker.redpanda.com/redpandadata/connect:4"
	// +optional
	Image string `json:"image,omitempty"`

	// SecretRefs maps Kubernetes Secret keys to environment variables injected
	// into the pipeline Pod. Reference them in RPC YAML as ${ENV_VAR}.
	// +optional
	SecretRefs []SecretRef `json:"secretRefs,omitempty"`

	// Stopped, when true, signals the operator to delete the pipeline pod
	// and keep it absent. The Pipeline CR (and its ConfigMap/PodMonitor)
	// remain in place. Toggle back to false to resume the pipeline.
	// F45: stop/run a pipeline without deleting the CR.
	// +kubebuilder:default=false
	// +optional
	Stopped bool `json:"stopped,omitempty"`

	// ClusterRef names a PipelineCluster in the same namespace. When set, the
	// pipeline is deployed as a stream into that cluster instead of getting its
	// own pod. Empty = one-pod model (unchanged). F47 Phase 2.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// ProjectRef opts this pipeline into a PipelineProject. Mutually exclusive
	// with clusterRef. When set, the operator places the pipeline onto the
	// project's managed PipelineCluster (<project>-cluster) and rewrites its
	// input/output per the project's routing table at stream-render time.
	// The Pipeline CR itself is never mutated. F50.2.
	// +optional
	ProjectRef *ProjectRef `json:"projectRef,omitempty"`
}

// ProjectRef references a PipelineProject in the same namespace.
type ProjectRef struct {
	// Name is the PipelineProject name in this namespace.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// PipelinePhase reports the high-level lifecycle stage of a Pipeline's pod.
// +kubebuilder:validation:Enum=Pending;Running;Failed;Stopped
type PipelinePhase string

const (
	PhasePending PipelinePhase = "Pending"
	PhaseRunning PipelinePhase = "Running"
	PhaseFailed  PipelinePhase = "Failed"
	PhaseStopped PipelinePhase = "Stopped"
)

// PipelineStatus defines the observed state of Pipeline.
type PipelineStatus struct {
	// +optional
	Phase PipelinePhase `json:"phase,omitempty"`

	// +optional
	PodName string `json:"podName,omitempty"`

	// AssignedCluster is the PipelineCluster the pipeline's stream currently runs
	// on; empty in pod mode. F47 Phase 2.
	// +optional
	AssignedCluster string `json:"assignedCluster,omitempty"`

	// AssignedInstance is the cluster pod (e.g. etl-small-1) hosting the stream;
	// empty in pod mode. F47 Phase 2.
	// +optional
	AssignedInstance string `json:"assignedInstance,omitempty"`

	// StreamID is the deployed stream ID (= pipeline name); empty in pod mode.
	// F47 Phase 2.
	// +optional
	StreamID string `json:"streamID,omitempty"`

	// StreamConfigHash is a hash of the stream config (placement + rendered body)
	// last successfully deployed. The reconciler skips re-deploying the stream on a
	// periodic resync when this still matches the desired config, avoiding an
	// unnecessary tear-down/recreate every reconcile.
	// +optional
	StreamConfigHash string `json:"streamConfigHash,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.status.podName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Pipeline is the Schema for the pipelines API.
type Pipeline struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PipelineSpec `json:"spec"`

	// +optional
	Status PipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipeline.
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
