package api

// Cluster-mode observability constants. F47 Phase 3a.
// streamLogField / streamMetricLabel are confirmed by the ds9s3 spike
// (see docs/test/f47-streams/phase3a-spike.md); change here if the spike differs.
const (
	streamLogField    = "stream" // JSON log field carrying the stream id (= pipeline name)
	streamMetricLabel = "stream" // Prometheus label carrying the stream id

	clusterLogPodWindow    = 2000 // pod-lines tailed for the cluster-mode log backlog
	clusterLogBacklogLines = 200  // max filtered backlog lines sent before live follow
)
