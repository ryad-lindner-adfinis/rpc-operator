package api

import (
	"context"
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func stripProjectManagedFields(items []rpcv1alpha1.PipelineProject) {
	for i := range items {
		items[i].ManagedFields = nil
	}
}

// validateProjectGraph lists the namespace's pipelines and validates the
// project's route graph against them. Returns nil when valid. Used by create
// and update so an invalid graph is never persisted (the controller still marks
// drift Degraded post-hoc — defense in depth).
func (s *Server) validateProjectGraph(
	ctx context.Context, c client.Client, ns string, p *rpcv1alpha1.PipelineProject,
) []ValidationError {
	var pipes rpcv1alpha1.PipelineList
	if err := c.List(ctx, &pipes, client.InNamespace(ns)); err != nil {
		return []ValidationError{{Path: "spec.routes", Message: "could not list pipelines: " + err.Error()}}
	}
	return ValidateProject(p, pipes.Items)
}

func (s *Server) handleListAllProjects(w http.ResponseWriter, r *http.Request) {
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineProjectList
	if err := c.List(r.Context(), &list); err != nil {
		writeK8sError(w, err)
		return
	}
	stripProjectManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleListNamespacedProjects(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineProjectList
	if err := c.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
		writeK8sError(w, err)
		return
	}
	stripProjectManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var p rpcv1alpha1.PipelineProject
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		writeK8sError(w, err)
		return
	}
	p.ManagedFields = nil
	writeJSON(w, http.StatusOK, &p)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var p rpcv1alpha1.PipelineProject
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	if p.Namespace != "" && p.Namespace != ns {
		writeJSONError(w, http.StatusBadRequest, "namespace mismatch",
			"body namespace must equal URL namespace")
		return
	}
	p.Namespace = ns
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	if verrs := s.validateProjectGraph(r.Context(), c, ns, &p); len(verrs) > 0 {
		writeValidationErrors(w, verrs)
		return
	}
	if err := c.Create(r.Context(), &p); err != nil {
		writeK8sError(w, err)
		return
	}
	p.ManagedFields = nil
	writeJSON(w, http.StatusCreated, &p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	var body rpcv1alpha1.PipelineProject
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var current rpcv1alpha1.PipelineProject
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.Spec = body.Spec
	if verrs := s.validateProjectGraph(r.Context(), c, ns, &current); len(verrs) > 0 {
		writeValidationErrors(w, verrs)
		return
	}
	if err := c.Update(r.Context(), &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.ManagedFields = nil
	writeJSON(w, http.StatusOK, &current)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var p rpcv1alpha1.PipelineProject
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		writeK8sError(w, err)
		return
	}
	if err := c.Delete(r.Context(), &p); err != nil {
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}
