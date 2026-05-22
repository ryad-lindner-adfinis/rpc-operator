package streams

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_EnsureStream_PUT(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient()
	if err := c.EnsureStream(context.Background(), srv.URL, "mypipe", "input: {}\n"); err != nil {
		t.Fatalf("EnsureStream: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/streams/mypipe" {
		t.Errorf("expected /streams/mypipe, got %s", gotPath)
	}
	if gotBody != "input: {}\n" {
		t.Errorf("body not forwarded, got %q", gotBody)
	}
	if gotCT != "application/x-yaml" {
		t.Errorf("expected Content-Type application/x-yaml, got %q", gotCT)
	}
}

func TestHTTPClient_EnsureStream_CreatesViaPOSTWhenAbsent(t *testing.T) {
	// Redpanda Connect streams API: PUT updates an existing stream and returns
	// 404 when it does not exist yet; POST creates it. EnsureStream must fall
	// back to POST on a PUT 404.
	var methods []string
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		postBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient()
	if err := c.EnsureStream(context.Background(), srv.URL, "new", "input: {}\n"); err != nil {
		t.Fatalf("EnsureStream should create via POST on PUT 404, got %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodPost {
		t.Errorf("expected PUT then POST, got %v", methods)
	}
	if postBody != "input: {}\n" {
		t.Errorf("config body not forwarded on POST, got %q", postBody)
	}
}

func TestHTTPClient_EnsureStream_ErrorOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewHTTPClient()
	if err := c.EnsureStream(context.Background(), srv.URL, "x", "input: {}\n"); err == nil {
		t.Errorf("expected error on 500, got nil")
	}
}

func TestHTTPClient_DeleteStream_404IsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewHTTPClient()
	if err := c.DeleteStream(context.Background(), srv.URL, "gone"); err != nil {
		t.Errorf("DeleteStream should treat 404 as success, got %v", err)
	}
}

func TestHTTPClient_ListStreams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"a":{},"b":{}}`))
	}))
	defer srv.Close()

	c := NewHTTPClient()
	got, err := c.ListStreams(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if _, ok := got["a"]; !ok {
		t.Errorf("expected stream a in %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 streams, got %d", len(got))
	}
}
