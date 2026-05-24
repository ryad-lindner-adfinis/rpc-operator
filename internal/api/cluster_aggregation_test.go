package api

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func TestInstanceOrdinal(t *testing.T) {
	cases := []struct {
		inst, cluster string
		want          int32
		ok            bool
	}{
		{"etl-0", "etl", 0, true},
		{"etl-12", "etl", 12, true},
		{"etl-small-0", "etl", 0, false}, // prefix is "etl-", remainder "small-0" not an int
		{"other-1", "etl", 0, false},
		{"etl-", "etl", 0, false},
		{"etl-x", "etl", 0, false},
	}
	for _, c := range cases {
		got, ok := instanceOrdinal(c.inst, c.cluster)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("instanceOrdinal(%q,%q)=(%d,%v) want (%d,%v)", c.inst, c.cluster, got, ok, c.want, c.ok)
		}
	}
}

func TestAggregateDistribution(t *testing.T) {
	cluster := &rpcv1alpha1.PipelineCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "etl", Namespace: "default"},
		Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 3},
		Status:     rpcv1alpha1.PipelineClusterStatus{Phase: rpcv1alpha1.ClusterPhaseReady, ReadyReplicas: 2},
	}
	ready := func(name string, r bool) corev1.Pod {
		cond := corev1.ConditionFalse
		if r {
			cond = corev1.ConditionTrue
		}
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: cond}}},
		}
	}
	pods := []corev1.Pod{ready("etl-0", true), ready("etl-1", false)} // etl-2 pod missing
	pipe := func(name, inst string) rpcv1alpha1.Pipeline {
		return rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status:     rpcv1alpha1.PipelineStatus{AssignedInstance: inst},
		}
	}
	pipes := []rpcv1alpha1.Pipeline{
		pipe("p2", "etl-0"), pipe("p1", "etl-0"), // unsorted on purpose
		pipe("p3", "etl-1"),
		pipe("p9", "etl-5"), // stale: ordinal >= replicas
		pipe("pX", ""),      // unassigned: ignored
	}

	got := aggregateDistribution(cluster, pods, pipes)

	if got.Cluster != "etl" || got.Namespace != "default" || got.Phase != "Ready" {
		t.Fatalf("header wrong: %+v", got)
	}
	if got.DesiredReplicas != 3 || got.ReadyReplicas != 2 {
		t.Fatalf("replica counts wrong: desired=%d ready=%d", got.DesiredReplicas, got.ReadyReplicas)
	}
	if len(got.Instances) != 3 {
		t.Fatalf("expected 3 instance slots, got %d", len(got.Instances))
	}
	// slot 0: ready, sorted [p1 p2]
	if got.Instances[0].Name != "etl-0" || !got.Instances[0].Ready ||
		len(got.Instances[0].AssignedPipelines) != 2 ||
		got.Instances[0].AssignedPipelines[0] != "p1" || got.Instances[0].AssignedPipelines[1] != "p2" {
		t.Errorf("slot0 wrong: %+v", got.Instances[0])
	}
	// slot 1: not ready, [p3]
	if got.Instances[1].Ready || len(got.Instances[1].AssignedPipelines) != 1 {
		t.Errorf("slot1 wrong: %+v", got.Instances[1])
	}
	// slot 2: missing pod -> not ready, empty (non-nil) list
	if got.Instances[2].Name != "etl-2" || got.Instances[2].Ready || got.Instances[2].AssignedPipelines == nil ||
		len(got.Instances[2].AssignedPipelines) != 0 {
		t.Errorf("slot2 wrong: %+v", got.Instances[2])
	}
	// stale placement p9
	if len(got.StalePlacements) != 1 || got.StalePlacements[0].Pipeline != "p9" ||
		got.StalePlacements[0].AssignedInstance != "etl-5" {
		t.Errorf("stale wrong: %+v", got.StalePlacements)
	}
}
