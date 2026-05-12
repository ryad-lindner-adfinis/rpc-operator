Rolle: Du bist ein Senior Software Architekt.
Ziel: Technisches Design und Implementierung des Redpanda Connect Operator (RPC Operator).
Vorgehen: Iterativ, Feature by Feature

## Anforderungen
Tech-Stack: Das System soll auf Kubernetes und Redpanda Connect Community (https://docs.redpanda.com/redpanda-connect/) basieren
Struktur des Dokuments:
Executive Summary: Ausführung von Redpanda Connect Pipelines in Kubernetes. UI gestütztes Monitoring und Konfiguration der Redpanda Connect Pipelines.
Diagramme: Mermaid-Code für die Architektur.

### Kontext

 Der RPC-Operator bietet eine flexible Möglichkeit Redpanda Connect (RPC) Pipelines zu konfigurieren und sie in Kubernetes betreiben zu können. Data Engineers bietet er über eine Web-Oberfläche die Möglichkeit alle Redpanda Connect Pipeline-Komponenten (Input, Processors, Output, etc.) graphisch oder als YAML zu konfigurieren. Der Data Engineer kann dann über ein einfaches Deploy seine konfigurierte Pipeline in einen Kubernetes Clsuter deployen und in der Web-Oberfläche monitoren.

## Redpanda Connect Operator – Architektur und Pipeline-Konfiguration in Kubernetes

### 1. Grundkonzept

Redpanda Connect basiert auf Benthos - ein deklarativer Data-Streaming-Service, der komplexe Datenpipelines durch einfache, verkettete, zustandslose Verarbeitungsschritte löst. Benthos garantiert at-least-once-Delivery ohne Persistenz der Nachrichten während der Verarbeitung und unterstützt eine Vielzahl von Connectors für Input/Output. Die Pipeline-Konfiguration erfolgt über eine YAML-Datei, die Input, Processor und Output definiert. Jede Konfiguration wird als Kubernetes Custom Resource (CR) gespeichert, und pro Konfiguration wird ein dedizierter Pod gestartet, der die Pipeline ausführt.

Quellen:
- https://github.com/redpanda-data/connect
- https://github.com/redpanda-data/benthos

### 2. Pipeline-Konfiguration

Beispielkonfiguration:
```
input:
  stdin: {}
pipeline:
  processors:
    - mapping: root = content().uppercase()
output:
  stdout: {}
```

Input/Output: Unterstützt u. a. stdin, stdout, aber auch Kafka, HTTP, Dateisysteme etc.
Processors: Ermöglichen Transformationen wie Mapping, Filterung, Aggregation etc.

### 3. Kubernetes-Integration

Custom Resource Definition (CRD): RPC-Operator nutzt eine CRD, um Pipeline-Konfigurationen als Kubernetes-Ressourcen zu speichern. Der RPC-Operator überwacht die CRs der CRDs und erstellt pro Konfiguration einen Pod, der die Pipeline ausführt.
Operator-Pattern: Der RPC-Operator ist ein Kubernetes Controller, der die Lebenszyklen der Pipelines verwaltet (Skalierung, Monitoring, Fehlerbehandlung)
Pods: Jeder Pipeline-Pod erhält eine Redpanda Connect Konfiguration (Input, Processor, Output) und führt die Pipeline als eigenständige Einheit mittels Redpanda Connect aus.

### 4. Vorteile

Einfache Bereitstellung: Pipelines werden als Kubernetes-Ressourcen verwaltet und können per kubectl deployt/monitored werden.
Skalierbarkeit: Jede Pipeline läuft in einem eigenen Pod, was horizontale Skalierung ermöglicht.
Resilienz: At-least-once-Delivery und Backpressure-Mechanismen sorgen für zuverlässige Datenverarbeitung.

## Spezifikationen

Alle Design-Entscheidung liegen in `docs/`. Lese immer relevante Specs bevor implementiert wird:

- `docs/prd.md` - Product Requirements mit Implementierungsstatus auf Release Ebene.
- `docs/architecture.md` - System Architektur, Tech Stack.
- `docs/adrs/*` - Dicision Log in Form von ADRs.
- `docs/prps/*` - Product Requirements Prompt, Feature Implementation Plan
