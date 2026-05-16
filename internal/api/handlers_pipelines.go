package api

import (
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

func (s *Server) handleListAll(w http.ResponseWriter, r *http.Request) {
	var list rpcv1alpha1.PipelineList
	if err := s.Client.List(r.Context(), &list); err != nil {
		writeK8sError(w, err)
		return
	}
	stripManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleListNamespaced(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var list rpcv1alpha1.PipelineList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
		writeK8sError(w, err)
		return
	}
	stripManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	var p rpcv1alpha1.Pipeline
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		writeK8sError(w, err)
		return
	}
	p.ManagedFields = nil
	writeJSON(w, http.StatusOK, &p)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var p rpcv1alpha1.Pipeline
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
	if verrs := ValidatePipeline(&p, s.Catalog); len(verrs) > 0 {
		writeValidationErrors(w, verrs)
		return
	}
	if err := s.Client.Create(r.Context(), &p); err != nil {
		writeK8sError(w, err)
		return
	}
	p.ManagedFields = nil
	writeJSON(w, http.StatusCreated, &p)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	var body rpcv1alpha1.Pipeline
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	body.Namespace = ns
	body.Name = name
	if verrs := ValidatePipeline(&body, s.Catalog); len(verrs) > 0 {
		writeValidationErrors(w, verrs)
		return
	}
	var current rpcv1alpha1.Pipeline
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.Spec = body.Spec
	if err := s.Client.Update(r.Context(), &current); err != nil {
		writeK8sError(w, err)
		return
	}
	current.ManagedFields = nil
	writeJSON(w, http.StatusOK, &current)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	var p rpcv1alpha1.Pipeline
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		writeK8sError(w, err)
		return
	}
	if err := s.Client.Delete(r.Context(), &p); err != nil {
		writeK8sError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	var p rpcv1alpha1.Pipeline
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	yamlText, err := render.RenderPipelineYAMLForDisplay(&p.Spec)
	if err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, "render failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": yamlText})
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var p rpcv1alpha1.Pipeline
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	verrs := ValidatePipeline(&p, s.Catalog)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":  len(verrs) == 0,
		"errors": verrs,
	})
}

func stripManagedFields(items []rpcv1alpha1.Pipeline) {
	for i := range items {
		items[i].ManagedFields = nil
	}
}
