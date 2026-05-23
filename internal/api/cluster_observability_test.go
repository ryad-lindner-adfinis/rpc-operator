package api

import (
	"strings"
	"testing"
)

func TestStreamLogMatch(t *testing.T) {
	cases := []struct {
		name, line, stream string
		want               bool
	}{
		{"match", `{"stream":"demo","msg":"hi"}`, "demo", true},
		{"other stream", `{"stream":"other","msg":"hi"}`, "demo", false},
		{"no stream field", `{"msg":"system line"}`, "demo", false},
		{"invalid json", `not json at all`, "demo", false},
		{"json array not object", `["demo"]`, "demo", false},
		{"stream not a string", `{"stream":123}`, "demo", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := streamLogMatch([]byte(c.line), "demo"); got != c.want {
				t.Errorf("streamLogMatch(%q)=%v want %v", c.line, got, c.want)
			}
		})
	}
}

func TestBuildMetricQuery(t *testing.T) {
	pod := buildMetricQuery("output_sent", "demo-pod", "")
	if pod != `rate(output_sent{pod="demo-pod"}[1m])` {
		t.Errorf("pod-mode query wrong: %q", pod)
	}
	cl := buildMetricQuery("output_sent", "etl-0", "demo")
	if cl != `rate(output_sent{pod="etl-0",stream="demo"}[1m])` {
		t.Errorf("cluster-mode query wrong: %q", cl)
	}
}

func TestFilterBacklog_CapsToLastN(t *testing.T) {
	// 5 matching lines for "demo" interleaved with other streams + system lines.
	var b strings.Builder
	b.WriteString(`{"stream":"demo","n":1}` + "\n")
	b.WriteString(`{"stream":"other","n":99}` + "\n")
	b.WriteString(`{"msg":"system"}` + "\n")
	b.WriteString(`{"stream":"demo","n":2}` + "\n")
	b.WriteString(`{"stream":"demo","n":3}` + "\n")
	b.WriteString(`{"stream":"demo","n":4}` + "\n")
	b.WriteString(`{"stream":"demo","n":5}` + "\n")

	got := filterBacklog(strings.NewReader(b.String()), "demo", 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 (capped), got %d", len(got))
	}
	// must be the LAST 3 matches (n:3,4,5)
	if !strings.Contains(string(got[0]), `"n":3`) ||
		!strings.Contains(string(got[2]), `"n":5`) {
		t.Errorf("expected last-3 matches, got %q", got)
	}
}
