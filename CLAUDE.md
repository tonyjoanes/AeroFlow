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
- [ ] kind cluster + namespaces
- [ ] NATS JetStream deployed, AEROFLOW stream
- [ ] go.work monorepo setup
- [ ] internal/messaging
- [ ] internal/models
- [ ] flight-service
- [ ] gate-service
- [ ] /health and /metrics on every service
- [ ] deploy manifests (Deployment, Service, ConfigMap, Secret, probes)

### Phase 2 — Full Event Chain + Ingress
- [ ] not started

### Phase 3 — Observability
- [ ] not started

### Phase 4 — Platform Layer
- [ ] not started

### Phase 5 — Stretch Goals
- [ ] not started
