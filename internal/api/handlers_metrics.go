package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetricsDatapoint is a single time-series point.
type MetricsDatapoint struct {
	T int64   `json:"t"` // Unix timestamp (seconds)
	V float64 `json:"v"` // value (msg/s)
}

// MetricsResponse is the API response for a metrics query.
type MetricsResponse struct {
	Query      string             `json:"query"`
	Unit       string             `json:"unit"`
	Datapoints []MetricsDatapoint `json:"datapoints"`
}

// knownQueries maps symbolic query names to PromQL templates.
// %s is replaced with the pod name.
var knownQueries = map[string]struct {
	tpl  string
	unit string
}{
	"throughput":           {`rate(redpanda_connect_output_sent{pod="%s"}[1m])`, "msg/s"},
	"error_rate":           {`rate(redpanda_connect_output_error{pod="%s"}[1m])`, "msg/s"},
	"input_rate":           {`rate(redpanda_connect_input_received{pod="%s"}[1m])`, "msg/s"},
	"processor_error_rate": {`rate(redpanda_connect_processor_error{pod="%s"}[1m])`, "msg/s"},
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	queryName := r.URL.Query().Get("query")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	step := r.URL.Query().Get("step")

	if step == "" {
		step = "30s"
	}
	now := time.Now().Unix()
	if end == "" {
		end = strconv.FormatInt(now, 10)
	}
	if start == "" {
		start = strconv.FormatInt(now-1800, 10)
	}

	q, ok := knownQueries[queryName]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "unknown_query",
			fmt.Sprintf("unknown query %q; valid: throughput, error_rate, input_rate, processor_error_rate", queryName))
		return
	}

	var pipe rpcv1alpha1.Pipeline
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &pipe); err != nil {
		writeK8sError(w, err)
		return
	}

	if pipe.Status.PodName == "" {
		writeJSONError(w, http.StatusConflict, "no_running_pod", "pipeline has no running pod")
		return
	}

	if s.PrometheusURL == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "prometheus_unavailable",
			"prometheus is not configured; set --prometheus-url")
		return
	}

	promQL := fmt.Sprintf(q.tpl, pipe.Status.PodName)
	datapoints, err := s.queryPrometheus(r.Context(), promQL, start, end, step)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "prometheus_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, MetricsResponse{
		Query:      queryName,
		Unit:       q.unit,
		Datapoints: datapoints,
	})
}

// queryPrometheus calls the Prometheus query_range API and returns normalized datapoints.
func (s *Server) queryPrometheus(ctx context.Context, promQL, start, end, step string) ([]MetricsDatapoint, error) {
	u, err := url.Parse(s.PrometheusURL + "/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("invalid prometheus url: %w", err)
	}
	q := u.Query()
	q.Set("query", promQL)
	q.Set("start", start)
	q.Set("end", end)
	q.Set("step", step)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %d", resp.StatusCode)
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Values [][2]json.RawMessage `json:"values"`
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

	// Sum all series into one timeline (handles pod restarts with multiple series).
	sums := map[int64]float64{}
	for _, series := range promResp.Data.Result {
		for _, point := range series.Values {
			var ts float64
			if err := json.Unmarshal(point[0], &ts); err != nil {
				continue
			}
			var valStr string
			if err := json.Unmarshal(point[1], &valStr); err != nil {
				continue
			}
			v, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			sums[int64(ts)] += v
		}
	}

	datapoints := make([]MetricsDatapoint, 0, len(sums))
	for t, v := range sums {
		datapoints = append(datapoints, MetricsDatapoint{T: t, V: v})
	}
	slices.SortFunc(datapoints, func(a, b MetricsDatapoint) int {
		if a.T < b.T {
			return -1
		}
		if a.T > b.T {
			return 1
		}
		return 0
	})
	return datapoints, nil
}
