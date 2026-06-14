# Changelog

All notable changes to this project are documented here.

## Feature — F52 Cache-Verwaltung in der Projekt-Map (UI) — 2026-06-14

### Hinzugefügt
- **F52 — Cache-Verwaltung in der Projekt-Map (UI):** Cache-Resources lassen sich in der
  tactical Map eines Projekts anlegen, bearbeiten und entfernen (Button „+ Cache", `CacheDrawer`
  für managed `natsKV` und custom YAML-Config). Die Map zeigt als grünen Datastore-Zylinder je
  Cache und einen gestrichelten Pfeil je Pipeline→Cache inkl. Operatoren (get/set/add/delete/exists).
  Nutzung wird aus `rawYAML`-Pipelines (cache-Prozessor) erkannt; nicht deklarierte Referenzen
  erscheinen als Warn-Phantomknoten. Reines UI-Feature auf Basis von F51 (keine Backend-Änderung).

## Feature — F51 Projekt-Cache-Resources — 2026-06-13

### Hinzugefügt
- **F51 — Projekt-Cache-Resources:** `PipelineProject.spec.cacheResources` macht Cache-Resources
  projektweit für alle Pipelines verfügbar. Variante `natsKV` (Operator legt das KV-Bucket
  `rpc-<projekt>-<name>` an und rendert die `nats_kv`-Config) oder `config` (beliebiger nativer
  Cache-Block, z. B. Redis, unverändert gepusht). Resources werden über die Streams-Resources-API
  (`POST /resources/cache/{label}`) auf jede Cluster-Instanz verteilt; Pod-Neustarts lösen ein
  Re-Push aus (Selbstheilung). Bare-ClusterRef-Pipelines erhalten das Feature bewusst nicht
  (siehe ADR-0005); Own-Pod-Pipelines nutzen `cache_resources` weiterhin direkt via `rawYAML`.
  Secret-Support ist für v1 deferred (F48-Funktionen sind der Reuse-Pfad).

## Feature — Pipeline Input/Output Connection Visibility — 2026-06-12

### Hinzugefügt
- **Pipeline Input/Output Connection Visibility:** Zwei neue read-only Endpunkte liefern den
  Live-Verbindungsstatus (`up`/`down`/`unknown`) eines Pipelines aus Prometheus-Gauges
  (`input_connection_up` / `output_connection_up`):
  - `GET …/pipelines/{name}/connections` (Einzelabfrage, Detailseite)
  - `GET …/pipelines/connections` (Batch-Abfrage für alle laufenden Pipelines im Namespace)
  Beide Endpunkte degradieren bei Prometheus-Fehler graceful auf `unknown` (kein 5xx).
  Im UI zeigt `PipelineDetail` eine zweispaltige Info-Box mit `ConnectionLights`-Dots
  (15s-Poll, nur bei `Running`); `PipelineList` zeigt einen farbigen Punkt pro Zeile
  (10s-Batch-Poll: grün = beide verbunden, rot = mind. eine Seite getrennt, grau = unbekannt).
  Keine Änderung an `Phase`, `Ready`, `StreamActive`, CRD oder Controller.
  Bekannte Grenze: Metrikname und `{pod,stream}`-Label-Schema werden erst auf ds9s3 bestätigt
  (ggf. nur Anpassung der Konstantennamen in `buildConnectionQuery`).

## Feature — Pipeline-Runnable-Health (`StreamActive`) — 2026-06-11

Cluster-/Stream-Pipelines melden jetzt den realen `active`-Status ihres Streams
(gelesen via `GET /streams/{id}` der Streams-API) als separate `StreamActive`-Condition.
Das ersetzt das implizite „Ready, weil `EnsureStream` 2xx zurückgab" durch ein Signal,
das den tatsächlichen Lauf-Zustand des Streams abbildet — der Ersatz für das bewusst
aufgegebene pod-globale `/ready` (siehe Poison-Stream-Kaskaden-Fix).

### Added

- **`StreamActive`-Condition** — `Ready`/`Phase` bleiben unverändert; die Condition trägt
  das Live-Signal: `True/Running` (aktiv), `False/StreamNotActive` (platziert, aber nicht
  aktiv), `False/StreamMissing` (Stream verschwunden) bzw. `Unknown/StatusUnavailable`
  (Status nicht lesbar — Platzierung bleibt bestehen).
- **Presence-Regel** — die Condition existiert genau dann, wenn die Pipeline eine
  Platzierung hält: gesetzt in `handleClusterAssigned`, entfernt in allen
  Unplacement-Pfaden (Stop/Pending/Failed/Fallback). Im Own-Pod-Modus wird sie nie gesetzt
  (dort ist Pod-Phase die Health-Quelle).
- **Bekannte Grenze (akzeptiert):** `active` bedeutet „lauffähig", nicht „verbunden" — eine
  fehlerhafte Output-URL hält `active == true`. Erreichbarkeit externer Dienste ist Teil 2
  (UI-only) und kein Pod-Readiness-/Reschedule-Signal.

## Fix — Einzelne fehlerhafte Cluster-Pipeline macht PipelineCluster nicht mehr ungesund — 2026-06-11

Eine einzelne Cluster-Pipeline mit ungültigem Ziel (z. B. unerreichbare Output-URL)
führte dazu, dass alle Streams auf den betroffenen Cluster-Instanzen auf `active: false`
fielen. Die Instanz-ReadinessProbe prüfte `/ready`, das den Health-Zustand sämtlicher
Streams meldet — eine einzige „vergiftete" Pipeline genügte, um die gesamte Instanz als
NotReady zu markieren. Daraufhin entfernte der Cluster-Controller gesunde Streams und
eskalierte die Migration in eine vollständige Kaskade. Zusätzlich lieferte der headless
Service nur Adressen gesunder Pods, sodass der Streams-API-Client keine Verbindung zur
Instanz aufbauen konnte. Jetzt prüft die ReadinessProbe `/ping` (nur HTTP-Erreichbarkeit)
statt `/ready`, und der headless Service publiziert auch NotReady-Adressen.

### Fixed

- **Poison-Stream-Kaskade eliminiert** — Die ReadinessProbe der Cluster-Instanz-Pods
  zeigt auf `/ping` (reine HTTP-Erreichbarkeit) statt auf `/ready` (Stream-Health aller
  Streams). Damit kann eine einzelne fehlerhafte Pipeline die Instanz nicht mehr als
  NotReady kippen; gesunde Pipelines laufen weiter.
- **Headless Service publiziert NotReady-Adressen** — `publishNotReadyAddresses: true`
  stellt sicher, dass der Streams-API-Client auch dann eine Verbindung zur Instanz
  aufbauen kann, wenn deren Readiness-Gate kurzzeitig nicht erfüllt ist.
- **Migrations-Kaskade verhindert** — Durch die zwei obigen Maßnahmen unterbricht ein
  einzelner fehlerhafter Stream nicht mehr die Platzierung oder Migration gesunder
  Streams auf demselben Cluster.

## Fix — Lastverteilung platziert Projekt-Pipelines wieder verteilt — 2026-06-10

Wird eine projektgebundene Pipeline gestoppt und neu gestartet, landete sie immer
wieder auf Cluster-Instanz 0 statt — wie erwartet — auf der am wenigsten belasteten
Instanz. Grund: Der Scheduler ermittelte die Instanz-Belegung (`loadByOrdinal`) über
`spec.clusterRef`, das bei Projekt-Pipelines (`spec.projectRef`) leer ist. Dadurch
zählte keine Projekt-Pipeline mit, die Last-Map war stets leer, und bei Gleichstand
gewann das kleinste Ordinal (0). Das betraf auch die Erstplatzierung — alle
Projekt-Pipelines wurden auf Instanz 0 gedrängt. (Schwester-Fix zur Cluster-Ansicht
vom 2026-06-09; selbe `spec.clusterRef`-Blindstelle.)

### Fixed

- **Lastverteilung zählt Projekt-Pipelines mit** — `loadByOrdinal` bestimmt die
  Belegung nun über die tatsächliche Platzierung (`status.assignedCluster`) statt
  über `spec.clusterRef`. Damit werden cluster- **und** projektgebundene Pipelines
  gezählt; neu gestartete Pipelines verteilen sich auf freie Instanzen.

## Fix — Projekt-Pipelines erscheinen in der Cluster-Ansicht — 2026-06-09

Eine projektgebundene Pipeline (`spec.projectRef`) läuft als Stream auf dem
projektverwalteten Cluster, tauchte aber nicht in dessen Cluster-Ansicht
(Instanz-Belegung) auf — die Zuordnung war nur an den Metriken zu erahnen. Grund:
die Mitgliedschaft wurde nur über `spec.clusterRef` bestimmt, das bei
Projekt-Pipelines leer ist. Jetzt zählen auch über den Status platzierte Pipelines.

### Fixed

- **Cluster-Ansicht zeigt Projekt-Pipelines** — `handleClusterInstances` nimmt
  eine Pipeline nun auch dann in die Instanz-Belegung auf, wenn sie über
  `status.assignedCluster` auf dem Cluster platziert ist (nicht nur bei
  `spec.clusterRef == <cluster>`). Die Instanz-Zuordnung selbst erfolgt
  unverändert über `status.assignedInstance`.

## Fix — Ungültige Stream-Config wird als Fehler sichtbar — 2026-06-09

Lehnt die Redpanda-Connect-Streams-API die Config einer cluster- oder
projektgebundenen Pipeline ab (HTTP 4xx, z. B. Lint-Fehler), wurde der Fehler
bislang nur als Reconciler-Fehler geloggt und endlos requeued — der Status blieb
auf dem alten Stand (z. B. `Stopped`) eingefroren, und der Nutzer sah nach „Run“
keine Fehlermeldung. Jetzt wird die Ablehnung in den Pipeline-Status geschrieben.

### Fixed

- **Stream-Config-Ablehnung im Status** — `EnsureStream` liefert bei einem 4xx
  einen typisierten `ConfigRejectedError`; der Reconciler setzt daraufhin
  `phase=Failed` mit Bedingung `Ready=False`/`StreamConfigInvalid` und der
  Lint-Meldung, statt in einer Fehlerschleife zu requeuen. 5xx/Transportfehler
  bleiben transient (weiterhin Retry).

## F50.4 Navigation — Editor-Rückkehr zum Ursprung + Projekt-Vorauswahl — 2026-06-04

Beim Bearbeiten einer aus einem Projekt geöffneten Pipeline führt „← Back“ jetzt
zurück zur Pipeline-Detailansicht (und von dort weiter zum Projekt); „Speichern“
kehrt zur aktualisierten Detailansicht zurück. Beim Anlegen einer neuen Pipeline
innerhalb eines Projekts ist das Projekt im YAML-Editor vorausgewählt, und
„Deploy“/„← Back“ führen zurück zum Projekt.

### Added

- **Editor-Ursprungs-Routing** — `App` merkt sich, woraus der Editor geöffnet
  wurde (`editorBackTarget`: Liste / Detail / Projekt); „← Back“ und „Speichern“
  kehren dorthin zurück. Ein Detail-Ursprung führt „Speichern“ zur frisch
  geladenen Detailansicht zurück (ursprünglicher `pipelineOrigin` bleibt erhalten).
- **Projekt-Vorauswahl im YAML-Editor** — `RawPipelineEditor` akzeptiert
  `initialProjectRef`; beim „+ Pipeline“ aus einem Projekt ist das Projekt im
  Dropdown vorausgewählt. Ein vorhandenes `editPipeline.projectRef` hat weiterhin
  Vorrang.

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
