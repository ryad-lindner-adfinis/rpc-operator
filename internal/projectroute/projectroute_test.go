package projectroute

import (
	"testing"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func sampleRoutes() []rpcv1alpha1.ProjectRoute {
	return []rpcv1alpha1.ProjectRoute{
		{Name: "fan-out", From: "ingest", To: []rpcv1alpha1.ProjectRouteTarget{
			{Pipeline: "warehouse"},
			{Pipeline: "alert", When: `this.level == "high"`},
		}},
	}
}

func TestNaming(t *testing.T) {
	if got := StreamName("orders", "fan-out"); got != "rpc-orders-fan-out" {
		t.Errorf("StreamName=%q", got)
	}
	if got := Subject("orders", "fan-out"); got != "rpc.orders.fan-out" {
		t.Errorf("Subject=%q", got)
	}
	if got := DurableName("orders", "fan-out", "alert"); got != "orders-fan-out-alert" {
		t.Errorf("DurableName=%q", got)
	}
	if got := NATSURL("orders", "ns"); got != "nats://orders-nats.ns.svc:4222" {
		t.Errorf("NATSURL=%q", got)
	}
}

func TestRoleOf(t *testing.T) {
	r := sampleRoutes()
	cases := map[string]Role{
		"ingest":    RoleSource,
		"warehouse": RoleSink,
		"alert":     RoleSink,
		"unrelated": RoleStandalone,
	}
	for pipe, want := range cases {
		if got := RoleOf(r, pipe); got != want {
			t.Errorf("RoleOf(%q)=%q want %q", pipe, got, want)
		}
	}
	mid := []rpcv1alpha1.ProjectRoute{
		{Name: "a", From: "x", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "m"}}},
		{Name: "b", From: "m", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "y"}}},
	}
	if got := RoleOf(mid, "m"); got != RoleMiddle {
		t.Errorf("RoleOf(m)=%q want middle", got)
	}
}

func TestPlanFor_SourceAndSink(t *testing.T) {
	proj := &rpcv1alpha1.PipelineProject{Spec: rpcv1alpha1.PipelineProjectSpec{Routes: sampleRoutes()}}
	proj.Name = "orders"

	src := PlanFor(proj, "ns", "ingest")
	if len(src.OutgoingSubjects) != 1 || src.OutgoingSubjects[0] != "rpc.orders.fan-out" {
		t.Errorf("source OutgoingSubjects=%v", src.OutgoingSubjects)
	}
	if len(src.Incoming) != 0 {
		t.Errorf("source should have no incoming")
	}

	alert := PlanFor(proj, "ns", "alert")
	if len(alert.Incoming) != 1 {
		t.Fatalf("alert Incoming=%v", alert.Incoming)
	}
	in := alert.Incoming[0]
	if in.Subject != "rpc.orders.fan-out" || in.Durable != "orders-fan-out-alert" || in.When != `this.level == "high"` {
		t.Errorf("alert incoming=%+v", in)
	}
}
