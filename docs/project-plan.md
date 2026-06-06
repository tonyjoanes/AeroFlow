Here's the full plan as text:

---

# AeroFlow — IDP Learning Project

**Stack**: Go · Kubernetes · NATS JetStream · Prometheus · Grafana · Loki · Tempo
**Estimated effort**: 12–16 weekends

---

## Phase 1 — Foundation (3–4 weekends)

Get a running Kubernetes cluster with NATS JetStream and two Go services exchanging real events. The goal is a working skeleton, not completeness.

**Kubernetes**
- Install `kind`, create cluster config
- Create namespaces: flights, baggage, gates, ground-ops, notifications, platform
- Deploy NATS with JetStream in its own namespace
- Create AEROFLOW stream capturing `aeroflow.>`
- Verify with `nats` CLI

**Go services**
- Monorepo Go module setup (`go.work`)
- `internal/messaging` — NATS client wrapper, pub/sub helpers
- `internal/models` — shared domain types (Flight, Gate, Bag)
- `flight-service` — REST API, publishes flight events
- `gate-service` — subscribes to flight events, assigns gates
- `/health` and `/metrics` endpoints on every service

**Deploy manifests**
- Deployment + Service for each Go service
- ConfigMap for NATS connection strings
- Secret for any credentials
- Liveness + readiness probes wired to `/health`

**Phase complete when**: flight lands → gate assigned via NATS, both services running in cluster, `/health` returns 200, `kubectl` shows pods Running.

---

## Phase 2 — Full Event Chain + Ingress (2–3 weekends)

Complete the domain event chain so a flight landing cascades through all services. Add NGINX ingress and TLS. Build the seed program.

**Remaining services**
- `baggage-service` — creates baggage job on LANDED event
- `carousel-service` — assigns carousel on BAGGAGE_STARTED
- `turnaround-service` — coordinates ground ops sequence
- `crew-dispatch-service` — assigns crew to aircraft
- `notification-service` — fan-out to all subscribers

**Ingress + TLS**
- Install NGINX ingress controller via Helm
- Path routing: `/api/*` → platform-api
- Host routing: `flights.aeroflow.local` etc
- cert-manager for self-signed TLS (learns the flow even if self-signed)
- Scale flight-service to 3 replicas, verify load balancing

**Seed program** (`hack/seed/`)
- Go binary that generates a realistic flight schedule (BA442, EK201...)
- Publishes events on a configurable schedule
- Burst mode for stress testing and making Grafana interesting
- Doubles as an integration test harness

**Phase complete when**: full event chain fires end to end, HTTPS via ingress working, seed generates a full day of flights, load balancing visible across replicas.

---

## Phase 3 — Observability (3–4 weekends)

Metrics first, then structured logs, then distributed traces. Each layer reveals something the previous one couldn't.

**Metrics (weeks 1–2)**
- Add `promhttp` to every Go service
- Custom counters: events published/consumed, errors
- Histograms: event processing latency per service
- Deploy `kube-prometheus-stack` Helm chart
- ServiceMonitor for each service namespace
- NATS JetStream exporter for consumer lag metrics
- Grafana: per-service dashboard + NATS queue depth board

**Logs (week 3)**
- Structured JSON logging in every service using `slog`
- Correlation IDs propagated through events
- Deploy Loki + Promtail via Helm
- Grafana: log panel correlated with metrics
- Demonstrate: error spike → log drill-down

**Traces (week 4)**
- OpenTelemetry SDK in each Go service
- Trace spans across: API call → NATS publish → consume → process
- Deploy Tempo via Grafana Helm chart
- Grafana: full trace waterfall for a single flight event chain
- Exemplars linking metrics → traces → logs

**Phase complete when**: Grafana shows live event rates per service, NATS consumer lag visible, single flight event traceable end to end, log query can find errors from a metric spike.

---

## Phase 4 — Platform Layer (2–3 weekends)

Build the IDP control plane: a Go API reading Kubernetes state, a simple ops dashboard, and the aeroctl CLI.

**platform-api (Go)**
- `internal/k8s` — client-go helpers
- `GET /services` — list pods/deployments per namespace
- `GET /health` — aggregate health across all namespaces
- `POST /rollout` — patch a Deployment image tag
- ServiceAccount with least-privilege RBAC
- In-cluster vs out-of-cluster config detection

**platform-ui (Go templates)**
- Served by platform-api as the same binary
- Service catalogue: all namespaces, pod counts, health status
- Live event feed from NATS subscription
- Links through to Grafana dashboards
- Flight board showing current flight states
- No JS framework — optionally htmx for live updates

**aeroctl CLI**
- Go binary using `cobra`
- `aeroctl services list`
- `aeroctl flights status`
- `aeroctl gates status`
- `aeroctl rollout [service] --image [tag]`
- Calls platform-api — same structural patterns as ppctl

**Phase complete when**: UI shows live cluster state, `aeroctl services list` works, rollout patches a Deployment, UI links through to Grafana.

---

## Phase 5 — Stretch Goals (ongoing)

Pick from these once the core is solid. Each is a self-contained learning spike.

**Scaling + resilience**
- HorizontalPodAutoscaler on flight-service
- Use seed burst mode to trigger scale-out, watch Grafana
- Resource requests + limits on all pods
- PodDisruptionBudget for critical services

**Multi-tenancy patterns**
- ResourceQuota per namespace
- LimitRange defaults
- NetworkPolicy: services can only reach NATS and platform-api
- Separate RBAC role per squad namespace

**Chaos + testing**
- Kill a consumer pod, verify NATS redelivers
- Simulate slow consumer, watch queue depth climb in Grafana
- Network partition with NetworkPolicy
- Go integration tests using seed + assertions on NATS state

---

## Suggested Weekend Rhythm

- Weekends 1–2: cluster up, Phase 1 services talking over NATS
- Weekends 3–4: full event chain, ingress, seed program
- Weekends 5–8: full observability stack
- Weekends 9–11: platform API, UI, aeroctl CLI
- Weekend 12+: stretch goals as interest develops

---

## Key Principles

- Build the seed program early and keep extending it — being able to fire `./hack/seed/seed --burst --flights 50` and watch everything light up in Grafana is what makes this feel real
- Don't rush to observability before the event chain is solid — Grafana graphs of nothing are not interesting
- `aeroctl` in Phase 4 directly transfers to `ppctl` — same cobra patterns, same CLI-calling-internal-API structure
- Keep the README good from day one — this is a portfolio piece
