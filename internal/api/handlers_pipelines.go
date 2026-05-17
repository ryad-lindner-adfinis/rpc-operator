package api

import (
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

func (s *Server) handleListAll(w http.ResponseWriter, r *http.Request) {
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineList
	if err := c.List(r.Context(), &list); err != nil {
		writeK8sError(w, err)
		return
	}
	stripManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleListNamespaced(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var list rpcv1alpha1.PipelineList
	if err := c.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
		writeK8sError(w, err)
		return
	}
	stripManagedFields(list.Items)
	writeJSON(w, http.StatusOK, map[string]any{"items": list.Items})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var p rpcv1alpha1.Pipeline
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
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
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	if err := c.Create(r.Context(), &p); err != nil {
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
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var current rpcv1alpha1.Pipeline
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

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var p rpcv1alpha1.Pipeline
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

// handleStop sets spec.stopped=true on the named Pipeline. The reconciler
// then deletes the pod and marks status.phase=Stopped.
// F45: idempotent — stopping an already-stopped pipeline is a no-op patch.
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.patchStopped(w, r, true)
}

// handleRun sets spec.stopped=false. The reconciler then recreates the pod.
// F45: idempotent — running an already-running pipeline is a no-op patch.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	s.patchStopped(w, r, false)
}

func (s *Server) patchStopped(w http.ResponseWriter, r *http.Request, stopped bool) {
	ns := r.PathValue("namespace")
	name := r.PathValue("name")
	c, err := s.clientForRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error", err.Error())
		return
	}
	var p rpcv1alpha1.Pipeline
	if err := c.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
		writeK8sError(w, err)
		return
	}
	patch := []byte(`{"spec":{"stopped":true}}`)
	if !stopped {
		patch = []byte(`{"spec":{"stopped":false}}`)
	}
	if err := c.Patch(r.Context(), &p, client.RawPatch(types.MergePatchType, patch)); err != nil {
		writeK8sError(w, err)
		return
	}
	p.ManagedFields = nil
	writeJSON(w, http.StatusOK, &p)
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
