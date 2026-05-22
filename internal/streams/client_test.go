package streams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_EnsureStream_PUT(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
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
