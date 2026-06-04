# Changelog

All notable changes to this project are documented here.

## F50.4 Navigation — Rückkehr zum Ursprung + Entwurfs-Erhalt — 2026-06-04

Öffnet man eine Pipeline aus einem Projekt (oder Cluster) heraus, führt „← Back“
jetzt zurück zur Projekt- bzw. Cluster-Ansicht statt zur Pipeline-Liste. Der
Abstecher in die Pipeline ist reversibel: ein laufender, noch nicht gespeicherter
Routen-Entwurf des Projekts bleibt erhalten, sodass man nahtlos weiterarbeiten kann.

### Added

- **Ursprungs-Routing** — `App` merkt sich einen Ursprung (`pipelineBackTarget`),
  wenn eine Pipeline-Detailansicht geöffnet wird; „← Back“ kehrt zum Projekt-,
  Cluster- oder Listen-Ursprung zurück (eine Ebene tief, per Design).
- **Entwurfs-Erhalt über den Abstecher** — der Routen-Entwurf
  (`draftRoutes`/`dirty`) liegt nun in `App` und wird als optionale Props an
  `ProjectDetail` gereicht; er überlebt das Aus-/Wiedereinhängen der Karte.
  Beim Betreten eines (anderen) Projekts wird der Entwurf zurückgesetzt, damit
  kein veralteter Entwurf zwischen Projekten durchschlägt.

### Changed

- **„Open pipeline“ verwirft den Entwurf nicht mehr** — das Öffnen einer
  Pipeline aus der Karte ist jetzt ein nicht-destruktiver Abstecher (kein
  „changes will be lost“-Dialog mehr). Back und „+ Pipeline“ fragen bei
  ungespeichertem Entwurf weiterhin nach.

## F50.3 Pipeline Projects — Taktische Karte: Entwurfsmodus — 2026-06-02

Router-Änderungen auf der taktischen Karte werden jetzt als clientseitiger,
sitzungsgebundener Entwurf gehalten und erst per **Save & deploy** committet.
Der Commit wird im Backend validiert: `handleCreateProject`/`handleUpdateProject`
prüfen den Routen-Graphen vor dem Schreiben und liefern `422` ohne zu
persistieren, wenn er ungültig ist (zugleich Härtung des Schreibpfads).

### Added

- **Routen-Entwurf** — Router anlegen/bearbeiten/entfernen mutiert nur einen
  lokalen `draftRoutes`-Zustand; die Karte zeigt die Änderungen sofort, ohne zu
  deployen. Eine „● Unsaved changes“-Pille signalisiert ungespeicherte Änderungen.
- **Save & deploy / Discard** — finaler, validierter Commit bzw. Verwerfen auf
  den Serverstand. Bei `422` listet ein roter Banner die verbatim
  Backend-Meldungen; der Entwurf bleibt zum Korrigieren erhalten. Save-Fehler
  (409/generisch) erscheinen in einem eigenen Banner, ohne die Karte samt
  Entwurf auszublenden.
- **Verlassen-Warnung** — Back, „Open pipeline“ und „+ Pipeline“ fragen bei
  ungespeichertem Entwurf nach (der Entwurf ist sitzungsgebunden).
- **Backend-Validierung beim Schreiben** — `ValidateProject` wird in den
  Projekt-Create/Update-Handlern aufgerufen; invalide Graphen werden nie
  persistiert (der Controller markiert Drift weiterhin nachgelagert `Degraded`).

### Notes

- Der Entwurf ist bewusst sitzungsgebunden (kein serverseitiger Draft); ein
  Reload oder Verlassen der Karte ohne Speichern verwirft ihn.

## F50.3 Pipeline Projects — UI — 2026-06-02

**Commits:** `5eecf6e`..HEAD (Branch `feat/f50.3-projects-ui`)

Bringt die Pipeline-Projects in die Weboberfläche: ein eigener Projects-Bereich
mit Listenansicht und einer taktischen Karte, die die `spec.routes[]` als Graph
aus Pipeline- und Router-Knoten rendert. Router lassen sich per Side-Drawer
anlegen/bearbeiten, und beide Editoren (Visual + Raw) sind projectRef-fähig.

> **Status:** ✅ Build- und testverifiziert (22 Vitest-Tests grün, `make test`
> + `go vet` grün). Der manuelle ds9s3-E2E-Click-Through (Projekt anlegen →
> Pipelines zuordnen → Router verdrahten → Karte prüfen → löschen) steht noch aus.

### Added

- **Projects-Navigation** — neuer Sidebar-Eintrag `Projects` (`FolderTree`-Icon)
  neben Pipelines und Clusters.
- **`ProjectList`** — Listenansicht aller PipelineProjects im Namespace mit
  Phase/Cluster-Status, 15s-Polling und 403→leer-Behandlung (Mode C).
- **Taktische Karte** (`ProjectDetail` + `TopologyCanvas`) — rendert das
  Route-Graph als SVG: blaue Pipeline-Rechtecke und bernsteinfarbene
  Router-Pillen, verbunden über Bezier-Kanten. Layout via reiner, unit-getesteter
  Kahn-Longest-Path-Schichtung (`topology.ts`) — keine `dagre`-Abhängigkeit.
  Fan-in erscheint natürlich als zwei Router-Knoten, die in dieselbe Pipeline münden.
- **Seiten-Panel** — Auswahl eines Knotens zeigt Details: für Router Subject
  (`rpc.<project>.<route>`), Stream (`rpc-<project>-<route>`), Producer und
  Targets-Tabelle; für Pipelines die Rolle und ein-/ausgehende Routen.
- **`RouterDrawer`** — Side-Drawer zum Anlegen/Bearbeiten einer Route
  (Name DNS-1123-validiert, `from`, mehrere `to[]` mit optionalem Bloblang-`when:`).
- **`ProjectForm`** — Neues-Projekt-Dialog (Name, Cluster-Instanzen, NATS-Storage).
- **projectRef-fähige Editoren** — Visual- und Raw-Editor erhalten ein
  Project-Dropdown (wechselseitig exklusiv zu `clusterRef`), ein Rollen-Badge
  (standalone/source/middle/sink), Managed-I/O-Banner für die vom Operator
  injizierten `nats_jetstream`-Blöcke und — im Raw-Editor — einen
  **Rendered (preview)**-Tab über `renderPipelineYAML`.
- **Backend-REST-Fläche** — `internal/api/handlers_projects.go`: List/Get/Create/
  Update/Delete für `pipelineprojects` (Reads mit anonymous-read-Fallback, Writes
  authentifiziert), gespiegelt nach `handlers_clusters.go`.
- **Geteilter `projectRole`-Helper** — `roleOf` + `outputManaged`/`inputManaged`,
  identisch genutzt in beiden Editoren und im Seiten-Panel.
- **Vitest-Harness** (ADR-0002) — Vitest + React Testing Library + jsdom + MSW,
  `make ui-test`-Target.

### Notes

- **Live-Edge-Metriken bewusst zurückgestellt** — es existiert keine
  Pro-Route-NATS-Metrikreihe; die Karte zeigt v1 ohne Durchsatz-Chips pro Kante.
  Optionen (Producer-Rate-Approximation jetzt bzw. NATS-JetStream-Exporter später)
  sind im Plan für einen späteren PRP dokumentiert.
- Die Editoren sind in Mode C nicht erreichbar (`App.handleEdit`/`handleNew`
  brechen bei `readOnly` früh ab); Schreib-Affordances in Liste, Karte und
  Seiten-Panel sind durchgängig an `readOnly` gekoppelt.

## F50.2 Pipeline Projects — Routes & I/O Rewriting — 2026-06-01

**Commits:** `481b608`..`d7059b0`

Wires the Pipelines of a PipelineProject together over NATS JetStream. A
project's `spec.routes[]` now provisions one JetStream stream per route and the
operator rewrites the input/output of project-attached Pipelines at render time
so messages flow `from → to[]` over NATS — users only author the non-managed
side of each pipeline.

> **Status:** ✅ E2E-verified on `ds9s3` (2026-06-01): fan-out delivery confirmed,
> predicate filtering observed in logs (`alert` stream — only `level=high` messages),
> cycle + I/O-conflict gates reject and recover, F48 secret substitution intact on
> a project pipeline, cascade delete with PVC retention verified.

### Added

- **`Pipeline.spec.projectRef`** — attaches a Pipeline to a PipelineProject. A
  projectRef pipeline runs as a stream on the project's managed cluster
  (`<project>-cluster`, the F47 path) rather than as a standalone pod.
- **`internal/projectroute`** — pure package: stable naming
  (`rpc-<project>-<route>` stream, `rpc.<project>.<route>` subject,
  `<project>-<route>-<pipeline>` durable, `nats://<project>-nats.<ns>.svc:4222`),
  per-pipeline role computation (standalone/source/middle/sink) and I/O plan,
  plus route-graph validation with exact rejection messages (missing/unknown
  pipeline references, cycles, predicate compilation, I/O conflicts, and
  producer/consumer mutual-exclusion).
- **Per-route JetStream streams** — the PipelineProject controller ensures one
  stream per valid route (with configurable retention; 24h / 1Gi defaults) and
  prunes stale streams; stream provisioning is skipped when the graph is invalid.
- **Route-driven I/O rewriting** (`render.ApplyProjectIO`) — injects
  `nats_jetstream` input/output (single or `broker` for fan-out/fan-in) and a
  consumer-side Bloblang `when:` filter (a `mapping` predicate, or a
  `switch`-on-`@nats_subject` for fan-in), applied **before** F48 secret
  substitution and `PUT /streams`. The Pipeline CR is never mutated.
- **`/render` preview** now shows the operator-injected NATS I/O for projectRef
  pipelines, so the UI preview matches the deployed stream config.
- **API + controller validation** — `ValidatePipeline` rejects
  `projectRef`+`clusterRef` together and allows an empty operator-managed I/O
  side; `ValidateProject` delegates to the shared graph validator. An invalid
  graph marks the Project `Degraded`, emits a `Warning InvalidRoutes` event, and
  surfaces a `RoutesValid=False` condition.
- **Sample:** `config/samples/rpc_v1alpha1_pipeline_projectref.yaml` — a routed
  `orders` project (ingest fans out to warehouse + a high-severity-only alert).

### Notes

- A true Kubernetes `ValidatingWebhookConfiguration` is intentionally **deferred**
  (no cert-manager/webhook infrastructure); validation is enforced at the
  controller and API layers instead.
- Route-change → immediate pipeline re-render via `Watches(&PipelineProject{})`
  is deferred; v1 relies on the existing resync interval.

## User Documentation v1 — 2026-05-31

**Commit:** `e51ae30`

First complete user documentation site for the RPC Operator. Covers all
operator features through v0.9, organized for two audiences: platform
administrators and pipeline authors.

### Sections

- **Introduction** — What the RPC Operator is, who should read what, and architecture overview with Mermaid diagrams
- **Getting Started** — Prerequisites, Helm install, first pipeline walkthrough, production readiness checklist
- **Operating the Operator** — Helm values reference, authentication modes (A/B/C), OIDC SSO, namespace allowlist, pull secrets, Prometheus integration, upgrades and uninstall
- **Authoring Pipelines** — Pipeline anatomy, secrets via secretKeyRef, deploying and redeploying, stop and re-run
- **PipelineClusters & Streams** — When to use a PipelineCluster, defining a cluster, running stream pipelines, migrating between clusters
- **Operations** — Reading logs (kubectl + WebSocket API + stream filtering), metrics and PodMonitor, troubleshooting guide
- **Reference** — Complete Pipeline CRD (16 fields), PipelineCluster CRD (8 fields), Helm values (47 rows), operator CLI flags (26 flags)

### Infrastructure

- MkDocs Material 9.5 with syntax highlighting, tabbed code blocks, Mermaid diagrams, full-text search
- CRD drift-check CI gate (`.drift-check-enabled`): fails if a CRD field is added to Go code without a corresponding `### fieldName` section in the reference docs
