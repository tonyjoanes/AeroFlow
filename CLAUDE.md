# AeroFlow — Working Notes

Learning project: an airport-operations IDP built with Go, Kubernetes, NATS JetStream,
and the Prometheus/Grafana/Loki/Tempo observability stack. See `docs/project-plan.md`
for the full phased plan.

## Goals for code quality

- This is a portfolio/learning piece — favour clarity and idiomatic Go over cleverness.
- Keep packages small and purpose-named; shared code lives under `internal/`.
- Every service exposes `/health` and `/metrics`.
- Run `gofmt`/`go vet` before committing; keep `go.work` in sync with new modules.

## Layout

```
flight-service/        REST API, publishes flight events
gate-service/          subscribes to flight events, assigns gates
internal/messaging/    NATS client wrapper, pub/sub helpers
internal/models/       shared domain types (Flight, Gate, Bag)
deploy/                Kubernetes manifests
docs/                  project plan and notes
```

## Progress

### Phase 1 — Foundation
- [x] kind cluster config + namespaces written (deploy/cluster/) — run locally per deploy/README.md
- [x] NATS JetStream + AEROFLOW stream (auto-created on connect; verified locally via Docker)
- [x] Go monorepo module setup (single module rather than go.work — see note below)
- [x] internal/messaging
- [x] internal/models
- [x] flight-service
- [x] gate-service
- [x] /health and /metrics on every service
- [x] event chain verified locally end to end: flight lands → gate assigned, correlation IDs match
- [x] deploy manifests (Deployment, Service, ConfigMap, probes, Dockerfiles) — see deploy/

Note: used a single Go module for the monorepo rather than go.work + per-service
modules — simpler for this size of project and `go.work` is gitignored anyway.

### Phase 2 — Full Event Chain + Ingress
- [x] baggage-service (LANDED → creates job → publishes BAGGAGE_STARTED)
- [x] carousel-service (BAGGAGE_STARTED → assigns carousel → publishes CAROUSEL_ASSIGNED)
- [x] turnaround-service (LANDED → starts ground ops → publishes TURNAROUND_STARTED)
- [x] crew-dispatch-service (LANDED → assigns crew → publishes CREW_ASSIGNED)
- [x] notification-service (fan-out subscriber on aeroflow.>)
- [x] seed program (hack/seed/) — --burst and --interval modes
- [x] NGINX ingress + TLS (cert-manager self-signed, deploy/ingress/)
- [ ] scale flight-service to 3 replicas

### Phase 3 — Observability
- [x] Custom Prometheus counters and histograms in every service (internal/metrics)
- [x] HTTP request metrics via instrumented mux wrapper
- [x] Structured JSON logging via slog with correlation IDs (all services)
- [x] OpenTelemetry tracing (internal/tracing) — spans across HTTP → NATS publish → consume
- [x] Trace context propagated through NATS message headers
- [x] No-op tracer when OTEL_EXPORTER_OTLP_ENDPOINT is unset (local dev safe)
- [x] kube-prometheus-stack Helm values (deploy/observability/)
- [x] ServiceMonitor for every service namespace
- [x] Loki + Promtail Helm values
- [x] Tempo Helm values (OTLP/HTTP receiver on :4318)
- [x] Grafana dashboards — event-chain (rates, latency, errors), service-detail (HTTP + heatmap + logs panel), trace-explorer (TraceQL examples)
- [x] Exemplars wired: Tempo traces→logs (Loki), Loki logs→traces (traceID derived field), Prometheus→Tempo service map

### Phase 4 — Platform Layer
- [x] platform-api/internal/k8s — client-go helpers (list deployments, aggregate health, patch image)
- [x] GET /api/services, GET /api/health, GET /api/flights, POST /api/rollout
- [x] platform-ui — Go templates served by platform-api (services catalogue, flight board, live event feed via SSE)
- [x] RBAC — ServiceAccount + ClusterRole (least-privilege: list/watch deployments+pods, patch deployments)
- [x] aeroctl CLI (cobra) — services list, health, flights status, rollout
- [x] deploy/platform — Deployment, Service, NodePort (30090→9000), RBAC manifests

### Phase 5 — Stretch Goals
- [x] Airport simulator (hack/simulator/) — fixed fleet of 20 aircraft cycling endlessly through SCHEDULED → BOARDING → DEPARTED → IN_FLIGHT → LANDED → turnaround → repeat. Configurable --speed multiplier. Live flight board updates via SSE.
- [ ] HPA on flight-service
- [ ] ResourceQuota / LimitRange per namespace
- [ ] NetworkPolicy
- [ ] Chaos testing
