// Package api serves an HTTP REST layer over the Pipeline CRD plus a static
// component catalog. v0.2 ships inside the operator binary; later milestones
// may split it into a dedicated process — keep this package strictly
// independent of internal/controller.
//
// SECURITY: v0.2 listens plain HTTP and performs no authn/authz. Front with
// an Ingress that terminates TLS and integrates with your OIDC provider until
// v0.6 ships in-process auth (docs/prd.md F20).
package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

// Server is an HTTP REST server that integrates with the controller-runtime Manager.
type Server struct {
	Addr          string
	Client        client.Client
	Clientset     *kubernetes.Clientset // for pod log streaming; nil in tests
	Catalog       *catalog.Catalog
	PrometheusURL string // empty = Prometheus not configured
	srv           *http.Server
}

// Compile-time check that Server implements manager.Runnable.
var _ manager.Runnable = (*Server)(nil)

// New constructs a Server. Returns an error if the embedded catalog fails to load.
func New(addr string, c client.Client, restCfg *rest.Config, prometheusURL string) (*Server, error) {
	cat, err := catalog.Default()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return &Server{Addr: addr, Client: c, Clientset: cs, Catalog: cat, PrometheusURL: prometheusURL}, nil
}

// Start implements manager.Runnable. Called by the manager once the cache is synced.
func (s *Server) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("api")
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.srv = &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("API server listening", "addr", s.Addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			log.Error(err, "API server shutdown error")
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// RegisterRoutesForTest exposes route registration for use in tests.
func (s *Server) RegisterRoutesForTest(mux *http.ServeMux) {
	s.registerRoutes(mux)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/pipelines", s.handleListAll)
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines", s.handleListNamespaced)
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}", s.handleGet)
	mux.HandleFunc("POST /api/v1/namespaces/{namespace}/pipelines", s.handleCreate)
	mux.HandleFunc("PUT /api/v1/namespaces/{namespace}/pipelines/{name}", s.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/namespaces/{namespace}/pipelines/{name}", s.handleDelete)
	mux.HandleFunc("POST /api/v1/pipelines/validate", s.handleValidate)
	mux.HandleFunc("POST /api/v1/pipelines/render", s.handleRender)
	mux.HandleFunc("GET /api/v1/catalog", s.handleCatalogList)
	mux.HandleFunc("GET /api/v1/catalog/{category}/{name}", s.handleCatalogGet)
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}/logs", s.handleLogStream)
	mux.HandleFunc("GET /api/v1/namespaces/{namespace}/pipelines/{name}/metrics", s.handleMetrics)

	// Serve the embedded SPA. Must come after all /api/v1/ routes (catch-all).
	sub, err := fs.Sub(StaticFiles, "static")
	if err != nil {
		panic("static embed broken: " + err.Error())
	}
	mux.Handle("/", http.FileServerFS(sub))
}
