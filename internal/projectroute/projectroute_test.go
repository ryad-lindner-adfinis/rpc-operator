package projectroute

import (
	"strings"
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

func pv(name, project string, hasIn, hasOut bool) PipelineView {
	return PipelineView{Name: name, ProjectName: project, HasInput: hasIn, HasOutput: hasOut}
}

func projWith(name string, routes []rpcv1alpha1.ProjectRoute) *rpcv1alpha1.PipelineProject {
	p := &rpcv1alpha1.PipelineProject{Spec: rpcv1alpha1.PipelineProjectSpec{Routes: routes}}
	p.Name = name
	return p
}

func msgs(errs []ProjectError) []string {
	out := make([]string, len(errs))
	for i := range errs {
		out[i] = errs[i].Message
	}
	return out
}

func hasMsg(errs []ProjectError, want string) bool {
	for _, m := range msgs(errs) {
		if m == want {
			return true
		}
	}
	return false
}

func TestValidateProject_Valid(t *testing.T) {
	proj := projWith("orders", sampleRoutes())
	pipes := map[string]PipelineView{
		"ingest":    pv("ingest", "orders", false, false),
		"warehouse": pv("warehouse", "orders", false, false),
		"alert":     pv("alert", "orders", false, false),
	}
	if errs := ValidateProject(proj, pipes); len(errs) != 0 {
		t.Fatalf("expected valid, got %v", msgs(errs))
	}
}

func TestValidateProject_MissingRefs(t *testing.T) {
	proj := projWith("orders", []rpcv1alpha1.ProjectRoute{
		{Name: "r", From: "ghost", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "void"}}},
	})
	errs := ValidateProject(proj, map[string]PipelineView{})
	if !hasMsg(errs, "route 'r' from='ghost': pipeline not found in project") {
		t.Errorf("missing from-error: %v", msgs(errs))
	}
	if !hasMsg(errs, "route 'r' to[0]='void': pipeline not found in project") {
		t.Errorf("missing to-error: %v", msgs(errs))
	}
}

func TestValidateProject_DuplicateName(t *testing.T) {
	proj := projWith("orders", []rpcv1alpha1.ProjectRoute{
		{Name: "dup", From: "a", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "b"}}},
		{Name: "dup", From: "a", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "c"}}},
	})
	pipes := map[string]PipelineView{
		"a": pv("a", "orders", false, false), "b": pv("b", "orders", false, false), "c": pv("c", "orders", false, false),
	}
	if !hasMsg(ValidateProject(proj, pipes), `route name "dup" is not unique`) {
		t.Errorf("missing uniqueness error")
	}
}

func TestValidateProject_Cycle(t *testing.T) {
	proj := projWith("orders", []rpcv1alpha1.ProjectRoute{
		{Name: "ab", From: "a", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "b"}}},
		{Name: "ba", From: "b", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "a"}}},
	})
	pipes := map[string]PipelineView{"a": pv("a", "orders", false, false), "b": pv("b", "orders", false, false)}
	errs := ValidateProject(proj, pipes)
	if !hasMsg(errs, "route graph contains a cycle: a → b → a") {
		t.Errorf("missing/odd cycle error: %v", msgs(errs))
	}
}

func TestValidateProject_BadPredicate(t *testing.T) {
	proj := projWith("orders", []rpcv1alpha1.ProjectRoute{
		{Name: "r", From: "a", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "b", When: "this.("}}},
	})
	pipes := map[string]PipelineView{"a": pv("a", "orders", false, false), "b": pv("b", "orders", false, false)}
	errs := ValidateProject(proj, pipes)
	found := false
	for _, m := range msgs(errs) {
		if strings.HasPrefix(m, "route 'r' to[0].when:") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing predicate parse error: %v", msgs(errs))
	}
}

func TestValidateProject_IOConflict(t *testing.T) {
	proj := projWith("orders", sampleRoutes())
	pipes := map[string]PipelineView{
		"ingest":    pv("ingest", "orders", false, true),
		"warehouse": pv("warehouse", "orders", true, false),
		"alert":     pv("alert", "orders", false, false),
	}
	errs := ValidateProject(proj, pipes)
	if !hasMsg(errs, "output is managed by the project's routes; remove it") {
		t.Errorf("missing output conflict: %v", msgs(errs))
	}
	if !hasMsg(errs, "input is managed by the project's routes; remove it") {
		t.Errorf("missing input conflict: %v", msgs(errs))
	}
}

func TestPlanFor_MiddleRole(t *testing.T) {
	routes := []rpcv1alpha1.ProjectRoute{
		{Name: "in", From: "x", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "m"}}},
		{Name: "out", From: "m", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "y"}}},
	}
	proj := projWith("orders", routes)
	plan := PlanFor(proj, "ns", "m")
	if len(plan.OutgoingSubjects) != 1 || plan.OutgoingSubjects[0] != "rpc.orders.out" {
		t.Errorf("middle Outgoing=%v", plan.OutgoingSubjects)
	}
	if len(plan.Incoming) != 1 || plan.Incoming[0].Subject != "rpc.orders.in" {
		t.Errorf("middle Incoming=%v", plan.Incoming)
	}
	if plan.IsEmpty() {
		t.Errorf("middle plan must not be empty")
	}
}

func TestIsEmpty_Standalone(t *testing.T) {
	proj := projWith("orders", sampleRoutes())
	plan := PlanFor(proj, "ns", "unrelated")
	if !plan.IsEmpty() {
		t.Errorf("standalone plan must be empty, got %+v", plan)
	}
}

func TestValidateProject_WrongProjectRef(t *testing.T) {
	// "alert" exists but claims a different project — must be treated as not in project.
	proj := projWith("orders", []rpcv1alpha1.ProjectRoute{
		{Name: "r", From: "ingest", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "alert"}}},
	})
	pipes := map[string]PipelineView{
		"ingest": pv("ingest", "orders", false, false),
		"alert":  pv("alert", "other-project", false, false),
	}
	errs := ValidateProject(proj, pipes)
	if !hasMsg(errs, "route 'r' to[0]='alert': pipeline not found in project") {
		t.Errorf("a pipeline in a different project must count as not-in-project: %v", msgs(errs))
	}
}
