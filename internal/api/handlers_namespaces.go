package api

import (
	"net/http"
	"slices"
)

// handleListNamespaces returns the operator's namespace allowlist as JSON.
// An empty list means cluster-wide; the UI falls back to free-text input in that case.
func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	ns := s.WatchNamespaces
	if ns == nil {
		ns = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"namespaces": ns})
}

// allowlist wraps a handler with a check that the URL's {namespace} path
// parameter is in the operator's allowlist. Empty allowlist disables the
// check (cluster-wide mode).
func (s *Server) allowlist(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(s.WatchNamespaces) == 0 {
			next(w, r)
			return
		}
		ns := r.PathValue("namespace")
		if ns == "" || !slices.Contains(s.WatchNamespaces, ns) {
			writeJSONError(w, http.StatusForbidden, "namespace_not_allowed",
				"namespace "+ns+" is outside the operator's allowlist")
			return
		}
		next(w, r)
	}
}
