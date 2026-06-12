package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConnState is the live connection state of one pipeline direction.
// Values: "up" | "down" | "unknown".
type ConnState = string

// ConnectionsResponse is the single-pipeline connection endpoint response.
type ConnectionsResponse struct {
	Input  ConnState `json:"input"`
	Output ConnState `json:"output"`
}

// BatchConnectionsResponse is the namespace-wide connection endpoint response.
// Key = pipeline name; only running pipelines are present.
type BatchConnectionsResponse map[string]ConnectionsResponse

// InstantSample is one result entry from a Prometheus instant (/api/v1/query) vector.
type InstantSample struct {
	Labels map[string]string
	Value  float64
}

// buildConnectionQuery returns the PromQL for a single pipeline's connection metric.
// stream="" → pod-only filter (own-pod mode); stream!="" → pod+stream (cluster mode).
func buildConnectionQuery(metric, pod, stream string) string {
	if stream == "" {
		return fmt.Sprintf(`min(%s{pod=%q})`, metric, pod)
	}
	return fmt.Sprintf(`min(%s{pod=%q,stream=%q})`, metric, pod, stream)
}

// buildBatchConnectionQuery returns the PromQL for multiple pods, grouped by pod+stream.
func buildBatchConnectionQuery(metric string, pods []string) string {
	return fmt.Sprintf(`min by (pod, stream) (%s{pod=~"^(%s)$"})`, metric, strings.Join(pods, "|"))
}

// queryPrometheusInstant issues an instant query against Prometheus and returns
// the vector results. Empty vector → empty slice. Non-success / transport → error.
func (s *Server) queryPrometheusInstant(ctx context.Context, promQL string) ([]InstantSample, error) {
	u, err := url.Parse(s.PrometheusURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("invalid prometheus url: %w", err)
	}
	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus instant query failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %d", resp.StatusCode)
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string  `json:"metric"`
				Value  [2]json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus error: %s", promResp.Error)
	}

	samples := make([]InstantSample, 0, len(promResp.Data.Result))
	for _, r := range promResp.Data.Result {
		var valStr string
		if err := json.Unmarshal(r.Value[1], &valStr); err != nil {
			continue
		}
		v, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		samples = append(samples, InstantSample{Labels: r.Metric, Value: v})
	}
	return samples, nil
}

// connStateFromSamples maps a vector result to a ConnState.
// Empty → "unknown", first value ≥1 → "up", else → "down".
func connStateFromSamples(samples []InstantSample) ConnState {
	if len(samples) == 0 {
		return "unknown"
	}
	if samples[0].Value >= 1 {
		return "up"
	}
	return "down"
}

// samplesMap converts a batch vector to a (pod+\x00+stream → ConnState) lookup.
// Missing stream label (own-pod metrics) is treated as "".
func samplesMap(samples []InstantSample) map[string]ConnState {
	m := make(map[string]ConnState, len(samples))
	for _, s := range samples {
		key := s.Labels["pod"] + "\x00" + s.Labels["stream"]
		if s.Value >= 1 {
			m[key] = "up"
		} else {
			m[key] = "down"
		}
	}
	return m
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	if s.PrometheusURL == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "prometheus_unavailable",
			"prometheus is not configured; set --prometheus-url")
		return
	}

	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var pipe rpcv1alpha1.Pipeline
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &pipe); err != nil {
		writeK8sError(w, err)
		return
	}

	pod := pipe.Status.PodName
	stream := ""
	if pipe.Status.AssignedInstance != "" {
		pod = pipe.Status.AssignedInstance
		stream = pipe.Name
	}
	if pod == "" {
		writeJSONError(w, http.StatusConflict, "no_running_pod", "pipeline has no running pod")
		return
	}

	inputSamples, inputErr := s.queryPrometheusInstant(r.Context(), buildConnectionQuery("input_connection_up", pod, stream))
	outputSamples, outputErr := s.queryPrometheusInstant(r.Context(), buildConnectionQuery("output_connection_up", pod, stream))

	inputState := "unknown"
	if inputErr == nil {
		inputState = connStateFromSamples(inputSamples)
	}
	outputState := "unknown"
	if outputErr == nil {
		outputState = connStateFromSamples(outputSamples)
	}

	writeJSON(w, http.StatusOK, ConnectionsResponse{Input: inputState, Output: outputState})
}

func (s *Server) handleNamespaceConnections(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")

	if s.PrometheusURL == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "prometheus_unavailable",
			"prometheus is not configured; set --prometheus-url")
		return
	}

	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}

	var pipelineList rpcv1alpha1.PipelineList
	if err := c.List(r.Context(), &pipelineList, client.InNamespace(ns)); err != nil {
		writeK8sError(w, err)
		return
	}

	type pipeInfo struct {
		name   string
		pod    string
		stream string // empty for own-pod mode
	}
	var running []pipeInfo
	for _, p := range pipelineList.Items {
		pod := p.Status.PodName
		stream := ""
		if p.Status.AssignedInstance != "" {
			pod = p.Status.AssignedInstance
			stream = p.Name
		}
		if pod != "" {
			running = append(running, pipeInfo{name: p.Name, pod: pod, stream: stream})
		}
	}

	result := BatchConnectionsResponse{}
	if len(running) == 0 {
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Unique pod names for the regex filter.
	podSet := map[string]struct{}{}
	for _, pi := range running {
		podSet[pi.pod] = struct{}{}
	}
	pods := make([]string, 0, len(podSet))
	for pod := range podSet {
		pods = append(pods, pod)
	}
	sort.Strings(pods) // deterministic PromQL for tests

	inputSamples, inputErr := s.queryPrometheusInstant(r.Context(), buildBatchConnectionQuery("input_connection_up", pods))
	outputSamples, outputErr := s.queryPrometheusInstant(r.Context(), buildBatchConnectionQuery("output_connection_up", pods))

	inMap := samplesMap(inputSamples)
	outMap := samplesMap(outputSamples)

	for _, pi := range running {
		key := pi.pod + "\x00" + pi.stream
		inputState := "unknown"
		outputState := "unknown"
		if inputErr == nil {
			if v, ok := inMap[key]; ok {
				inputState = v
			}
		}
		if outputErr == nil {
			if v, ok := outMap[key]; ok {
				outputState = v
			}
		}
		result[pi.name] = ConnectionsResponse{Input: inputState, Output: outputState}
	}

	writeJSON(w, http.StatusOK, result)
}
