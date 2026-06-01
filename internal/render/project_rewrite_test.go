package render

import (
	"strings"
	"testing"
)

const url = "nats://orders-nats.ns.svc:4222"

func TestApplyProjectIO_EmptyPlanNoChange(t *testing.T) {
	in := "input:\n  generate: {}\noutput:\n  stdout: {}\n"
	out, err := ApplyProjectIO(in, ProjectIOPlan{NATSURL: url})
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("empty plan changed YAML:\n%s", out)
	}
}

func TestApplyProjectIO_SourceReplacesOutput(t *testing.T) {
	in := "input:\n  generate: {}\noutput:\n  stdout: {}\n"
	out, err := ApplyProjectIO(in, ProjectIOPlan{NATSURL: url, OutgoingSubjects: []string{"rpc.orders.fan-out"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nats_jetstream") || !strings.Contains(out, "rpc.orders.fan-out") {
		t.Errorf("output not rewritten:\n%s", out)
	}
	if strings.Contains(out, "stdout") {
		t.Errorf("user output not replaced:\n%s", out)
	}
	if !strings.Contains(out, "generate") {
		t.Errorf("input should be untouched:\n%s", out)
	}
}

func TestApplyProjectIO_MultiOutputFanOutBroker(t *testing.T) {
	out, err := ApplyProjectIO("output:\n  stdout: {}\n",
		ProjectIOPlan{NATSURL: url, OutgoingSubjects: []string{"rpc.orders.a", "rpc.orders.b"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fan_out") || !strings.Contains(out, "rpc.orders.a") || !strings.Contains(out, "rpc.orders.b") {
		t.Errorf("expected fan_out broker:\n%s", out)
	}
}

func TestApplyProjectIO_SinkSingleWithPredicate(t *testing.T) {
	out, err := ApplyProjectIO("input:\n  stdin: {}\npipeline:\n  processors: []\noutput:\n  stdout: {}\n",
		ProjectIOPlan{NATSURL: url, Incoming: []IncomingRoute{
			{Subject: "rpc.orders.fan-out", Durable: "orders-fan-out-alert", When: `this.level == "high"`},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "durable: orders-fan-out-alert") {
		t.Errorf("durable missing:\n%s", out)
	}
	if !strings.Contains(out, `root = if !(this.level == "high")`) {
		t.Errorf("predicate mapping missing:\n%s", out)
	}
	if strings.Contains(out, "stdin") {
		t.Errorf("user input not replaced:\n%s", out)
	}
}

func TestApplyProjectIO_PredicatePrependsBeforeUserProcessors(t *testing.T) {
	// A consumer with an existing user processor (a mapping). The injected
	// predicate filter must come FIRST so it drops messages before user logic runs.
	in := "input:\n  stdin: {}\n" +
		"pipeline:\n  processors:\n    - mapping: root.user = \"kept\"\n" +
		"output:\n  stdout: {}\n"
	out, err := ApplyProjectIO(in, ProjectIOPlan{NATSURL: url, Incoming: []IncomingRoute{
		{Subject: "rpc.orders.fan-out", Durable: "orders-fan-out-sink", When: "this.ok"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	filterIdx := strings.Index(out, `root = if !(this.ok)`)
	userIdx := strings.Index(out, `root.user = "kept"`)
	if filterIdx < 0 || userIdx < 0 {
		t.Fatalf("both processors must be present:\n%s", out)
	}
	if filterIdx > userIdx {
		t.Errorf("predicate filter must come BEFORE the user processor:\n%s", out)
	}
}

func TestApplyProjectIO_FanInSwitchOnSubject(t *testing.T) {
	out, err := ApplyProjectIO("output:\n  stdout: {}\n",
		ProjectIOPlan{NATSURL: url, Incoming: []IncomingRoute{
			{Subject: "rpc.orders.a", Durable: "orders-a-sink", When: `this.x > 1`},
			{Subject: "rpc.orders.b", Durable: "orders-b-sink"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "broker") || !strings.Contains(out, "inputs") {
		t.Errorf("fan-in should use input broker:\n%s", out)
	}
	if !strings.Contains(out, "switch") || !strings.Contains(out, `@nats_subject == "rpc.orders.a"`) {
		t.Errorf("switch-on-subject missing:\n%s", out)
	}
	if strings.Contains(out, `@nats_subject == "rpc.orders.b"`) {
		t.Errorf("unfiltered route must not get a switch case:\n%s", out)
	}
}
