package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHandlerRender_Success(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/render",
		"application/json",
		bytes.NewReader(validPipelineBody("test", "default")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, want := range []string{"input:", "generate:", "pipeline:", "processors:", "output:", "stdout:"} {
		if !strings.Contains(body.YAML, want) {
			t.Errorf("rendered YAML missing %q\n--- output ---\n%s", want, body.YAML)
		}
	}
	if strings.Contains(body.YAML, "http:") || strings.Contains(body.YAML, "4195") {
		t.Errorf("rendered YAML must not include http probe block\n--- output ---\n%s", body.YAML)
	}
}

func TestHandlerRender_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/render",
		"application/json",
		strings.NewReader("not-json"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandlerRender_RenderError(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// rawYAML that is not a YAML mapping triggers a render error in injectHTTPConfig.
	body := []byte(`{
		"apiVersion":"rpc.operator.io/v1alpha1","kind":"Pipeline",
		"metadata":{"name":"x","namespace":"default"},
		"spec":{"rawYAML":"- just\n- a\n- list\n"}
	}`)

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/render",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestHandlerRender_RawYAML(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := []byte(`{
		"apiVersion":"rpc.operator.io/v1alpha1","kind":"Pipeline",
		"metadata":{"name":"x","namespace":"default"},
		"spec":{"rawYAML":"input:\n  stdin: {}\noutput:\n  stdout: {}\n"}
	}`)

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/render",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var decoded struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(decoded.YAML, "stdin:") || !strings.Contains(decoded.YAML, "stdout:") {
		t.Errorf("rendered YAML missing user content\n--- output ---\n%s", decoded.YAML)
	}
	if strings.Contains(decoded.YAML, "http:") {
		t.Errorf("rendered YAML must not include http probe block\n--- output ---\n%s", decoded.YAML)
	}
}
