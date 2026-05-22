/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/controller"
	"github.com/insidegreen/rpc-operator-claude/internal/streams"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(rpcv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var apiAddr string
	var prometheusURL string
	var watchNamespacesRaw string
	var authEnabled bool
	var anonymousRead bool
	var anonymousLogs bool
	var oidcIssuer string
	var oidcClientID string
	var oidcScopesRaw string
	var oidcRedirectURL string
	var oidcUIRedirectURL string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.StringVar(&apiAddr, "api-bind-address", ":8082",
		"Address the REST API listens on. Empty string disables the API server.")
	flag.StringVar(&prometheusURL, "prometheus-url", "",
		"Prometheus base URL for PromQL queries (e.g. http://prometheus:9090). Empty disables metrics.")
	flag.StringVar(&watchNamespacesRaw, "watch-namespaces", "",
		"Comma-separated namespace allowlist for operator cache and API. Empty = cluster-wide.")
	flag.BoolVar(&authEnabled, "auth-enabled", true,
		"Enable Bearer-Token authentication (F20a). false = v0.7-equivalent (operator-SA serves all requests).")
	flag.BoolVar(&anonymousRead, "anonymous-read-enabled", false,
		"F42: Allow unauthenticated GETs on pipelines/catalog/namespaces. Requires --auth-enabled=true.")
	flag.BoolVar(&anonymousLogs, "anonymous-logs-enabled", false,
		"F42: Allow unauthenticated log-stream WS connections. "+
			"Requires --auth-enabled=true. Log content may contain payloads/secrets.")
	flag.StringVar(&oidcIssuer, "oidc-issuer", "",
		"F20b: OIDC issuer URL (empty = OIDC disabled; F20a token-paste remains).")
	flag.StringVar(&oidcClientID, "oidc-client-id", "",
		"F20b: OIDC client ID registered at the IdP. Public client (PKCE).")
	flag.StringVar(&oidcScopesRaw, "oidc-scopes", "openid,email,offline_access",
		"F20b: comma-separated OIDC scopes. offline_access is required for refresh tokens.")
	flag.StringVar(&oidcRedirectURL, "oidc-redirect-url", "",
		"F20b: full public callback URL (https://<host>/api/v1/auth/callback). Must match the IdP registration.")
	flag.StringVar(&oidcUIRedirectURL, "oidc-ui-redirect-url", "",
		"F20b: where the browser lands after callback success. Empty defaults to '/' (same-origin SPA).")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	watchNamespaces := parseWatchNamespaces(watchNamespacesRaw)

	if !authEnabled {
		setupLog.Info("AUTH DISABLED — operator-SA serves all requests")
	}
	if authEnabled && anonymousRead {
		setupLog.Info("ANONYMOUS READS ENABLED — unauthenticated GETs allowed (F42)")
	}
	if authEnabled && anonymousLogs {
		setupLog.Info("ANONYMOUS LOG STREAMING ENABLED — unauthenticated /logs allowed (F42)")
	}

	var oidcCfg *api.OIDCConfig
	if oidcIssuer != "" {
		if !authEnabled {
			setupLog.Error(nil, "OIDC requires --auth-enabled=true (F20b is additive on F20a)")
			os.Exit(1)
		}
		oidcCfg = &api.OIDCConfig{
			Issuer:        oidcIssuer,
			ClientID:      oidcClientID,
			Scopes:        parseCSV(oidcScopesRaw),
			RedirectURL:   oidcRedirectURL,
			UIRedirectURL: oidcUIRedirectURL,
		}
		setupLog.Info("OIDC enabled (F20b)", "issuer", oidcIssuer, "clientID", oidcClientID)
	} else {
		setupLog.Info("OIDC disabled — F20a token-paste only")
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "3aedf8a9.operator.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}
	if len(watchNamespaces) > 0 {
		defaults := make(map[string]cache.Config, len(watchNamespaces))
		for _, ns := range watchNamespaces {
			defaults[ns] = cache.Config{}
		}
		mgrOpts.Cache = cache.Options{DefaultNamespaces: defaults}
		setupLog.Info("Cache restricted to namespaces", "namespaces", watchNamespaces)
	} else {
		setupLog.Info("Cache is cluster-wide (no --watch-namespaces set)")
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	if err := (&controller.PipelineReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Streams: streams.NewHTTPClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "pipeline")
		os.Exit(1)
	}

	if err := (&controller.PipelineClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "pipelinecluster")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if apiAddr != "" {
		apiSrv, err := api.New(
			apiAddr, mgr.GetClient(), mgr.GetConfig(), mgr.GetScheme(),
			prometheusURL, watchNamespaces,
			authEnabled, anonymousRead, anonymousLogs,
			oidcCfg,
		)
		if err != nil {
			setupLog.Error(err, "Failed to create API server")
			os.Exit(1)
		}
		if err := mgr.Add(apiSrv); err != nil {
			setupLog.Error(err, "Failed to register API server with manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

// parseCSV splits a comma-separated string, trims whitespace, drops empty
// entries. Returns nil for empty input so callers can pass it straight to
// an `omitempty`-style consumer (here: oauth2.Config.Scopes).
func parseCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseWatchNamespaces splits a comma-separated namespace list, trims whitespace,
// and drops empty entries. Returns nil for an empty input so callers can use
// len() == 0 as the cluster-wide signal.
func parseWatchNamespaces(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
