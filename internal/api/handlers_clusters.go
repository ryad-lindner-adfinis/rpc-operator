package api

import (
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func stripClusterManagedFields(items []rpcv1alpha1.PipelineCluster) {
	for i := range items {
		items[i].ManagedFields = nil
	}
}

func (s *Server) handleListAllClusters(w http.ResponseWriter, r *http.Request) {
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineClusterList
	if err := c.List(r.Context(), &list); err != nil {
		writeK8sError(w, err)
		return
	}
	stripClusterManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleListNamespacedClusters(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineClusterList
	if err := c.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
		writeK8sError(w, err)
		return
	}
	stripClusterManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var cl rpcv1alpha1.PipelineCluster
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &cl); err != nil {
		writeK8sError(w, err)
		return
	}
	cl.ManagedFields = nil
	writeJSON(w, http.StatusOK, &cl)
}

func (s *Server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var cl rpcv1alpha1.PipelineCluster
	if err := json.NewDecoder(r.Body).Decode(&cl); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	if cl.Namespace != "" && cl.Namespace != ns {
		writeJSONError(w, http.StatusBadRequest, "namespace mismatch",
			"body namespace must equal URL namespace")
		return
	}
	cl.Namespace = ns
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	if err := c.Create(r.Context(), &cl); err != nil {
		writeK8sError(w, err)
		return
	}
	cl.ManagedFields = nil
	writeJSON(w, http.StatusCreated, &cl)
}

func (s *Server) handleUpdateCluster(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	var body rpcv1alpha1.PipelineCluster
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var current rpcv1alpha1.PipelineCluster
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.Spec = body.Spec
	if err := c.Update(r.Context(), &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.ManagedFields = nil
	writeJSON(w, http.StatusOK, &current)
}

func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var cl rpcv1alpha1.PipelineCluster
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &cl); err != nil {
		writeK8sError(w, err)
		return
	}
	if err := c.Delete(r.Context(), &cl); err != nil {
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}
