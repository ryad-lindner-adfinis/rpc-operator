package api

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// clusterLabelKey is the pod label the controller stamps on every cluster
// instance (mirror of internal/controller's const; the api package can't import
// the controller package). F47 Phase 3b.
const clusterLabelKey = "rpc.operator.io/cluster"

// ClusterInstance is one desired instance slot and the pipelines placed on it.
type ClusterInstance struct {
	Name              string   `json:"name"`
	Ordinal           int32    `json:"ordinal"`
	Ready             bool     `json:"ready"`
	AssignedPipelines []string `json:"assignedPipelines"`
}

// StalePlacement is a pipeline assigned to an ordinal outside the desired range
// (Phase-2 scale-down transient, awaiting reschedule).
type StalePlacement struct {
	Pipeline         string `json:"pipeline"`
	AssignedInstance string `json:"assignedInstance"`
}

// ClusterDistribution is the API's derived per-instance view of a cluster.
type ClusterDistribution struct {
	Cluster         string            `json:"cluster"`
	Namespace       string            `json:"namespace"`
	Phase           string            `json:"phase"`
	DesiredReplicas int32             `json:"desiredReplicas"`
	ReadyReplicas   int32             `json:"readyReplicas"`
	Instances       []ClusterInstance `json:"instances"`
	StalePlacements []StalePlacement  `json:"stalePlacements"`
}

// instanceOrdinal parses the trailing ordinal from a StatefulSet pod/instance
// name like "etl-3" given the cluster name "etl". Returns false if name does
// not match exactly "<cluster>-<n>" with n a non-negative integer.
func instanceOrdinal(instanceName, cluster string) (int32, bool) {
	prefix := cluster + "-"
	if !strings.HasPrefix(instanceName, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(instanceName[len(prefix):])
	if err != nil || n < 0 {
		return 0, false
	}
	return int32(n), true
}

// isPodReady reports whether the pod has a Ready condition set to True.
func isPodReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// aggregateDistribution derives the per-instance distribution from the cluster
// CR, its instance pods, and the pipelines that reference it. Pure: no I/O. The
// caller is responsible for passing only pods labelled for this cluster and only
// pipelines whose clusterRef == cluster.Name. F47 Phase 3b.
func aggregateDistribution(cluster *rpcv1alpha1.PipelineCluster, pods []corev1.Pod,
	pipelines []rpcv1alpha1.Pipeline) ClusterDistribution {

	name := cluster.Name
	replicas := cluster.Spec.Replicas

	readyByOrdinal := map[int32]bool{}
	for i := range pods {
		if ord, ok := instanceOrdinal(pods[i].Name, name); ok {
			readyByOrdinal[ord] = isPodReady(&pods[i])
		}
	}

	assignedByOrdinal := map[int32][]string{}
	stale := []StalePlacement{}
	for i := range pipelines {
		inst := pipelines[i].Status.AssignedInstance
		if inst == "" {
			continue
		}
		ord, ok := instanceOrdinal(inst, name)
		if !ok {
			continue
		}
		if ord >= replicas {
			stale = append(stale, StalePlacement{Pipeline: pipelines[i].Name, AssignedInstance: inst})
			continue
		}
		assignedByOrdinal[ord] = append(assignedByOrdinal[ord], pipelines[i].Name)
	}

	instances := make([]ClusterInstance, 0, replicas)
	for ord := int32(0); ord < replicas; ord++ {
		names := assignedByOrdinal[ord]
		if names == nil {
			names = []string{}
		}
		sort.Strings(names)
		instances = append(instances, ClusterInstance{
			Name:              fmt.Sprintf("%s-%d", name, ord),
			Ordinal:           ord,
			Ready:             readyByOrdinal[ord],
			AssignedPipelines: names,
		})
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Pipeline < stale[j].Pipeline })

	return ClusterDistribution{
		Cluster:         name,
		Namespace:       cluster.Namespace,
		Phase:           string(cluster.Status.Phase),
		DesiredReplicas: replicas,
		ReadyReplicas:   cluster.Status.ReadyReplicas,
		Instances:       instances,
		StalePlacements: stale,
	}
}
