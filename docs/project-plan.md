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

## Phase 2.5 — Web Dashboards (2–3 weekends)

Build two real-time web dashboards consuming NATS events: one for passengers/public, one for operations staff. Both use Go templates with WebSocket or SSE for live updates.

**flight-board-web** (Separate Go service)
- Real-time flight status board: departures, arrivals, delays, gates
- Baggage carousel assignments and status
- Gate information: current assignment, next flight
- WebSocket subscription to NATS stream for live updates
- Simple HTML/CSS responsive design (works on airport displays and phones)
- Route: `/board` main display, `/status/:flight-id` for flight details
- Consume events: FLIGHT_LANDED, GATE_ASSIGNED, BAGGAGE_STARTED, CAROUSEL_ASSIGNED

**ops-dashboard-web** (Separate Go service or integrated with platform-api)
- Unified view for ramp/ground operations
- Active flights: gate status, crew assignments, baggage queues
- Carousel status: which bags on which carousel, completion %
- Turnaround metrics: ground time elapsed, next flight readiness
- Service health per namespace (link to platform-api `/health`)
- Event log: recent events with timestamps and correlation IDs
- WebSocket to NATS for live updates; same events plus CREW_DISPATCHED, TURNAROUND_COMPLETE

**Shared patterns**
- Both services subscribe to NATS from Go (shared `internal/messaging`)
- Broadcast events to connected clients via WebSocket (or SSE for simpler approach)
- Graceful reconnect on connection loss
- No JavaScript framework — vanilla JS for interactivity where needed

**Phase complete when**: Flight board displays live flight status updated in real-time, ops dashboard shows full ground operations view with baggage/carousel queues, both accessible via ingress, seed program updates visible immediately on both dashboards.

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

## Phase 4.5 — Self-Service Layer (2–3 weekends)

Empower developers to provision and manage their own services without platform team involvement. Turn the ops API into a true IDP.

**Service templates**
- Go module templates for Go+NATS service
- Scaffold generator: `aeroctl service create --name my-service --template go-nats`
- Creates service directory structure, Dockerfile, Kubernetes manifests, basic handlers
- Templates include stub messaging, `/health`, `/metrics`, NATS subscription

**Self-service provisioning**
- `aeroctl namespace create [squad-name]` — creates namespace with ResourceQuota, LimitRange, ServiceAccount
- Auto-provision NATS consumer groups per service
- Auto-wire RBAC: service SA can only read its configmap and write NATS
- Generate ConfigMap with cluster endpoints (NATS, prometheus, etc)

**Deployment workflow**
- `aeroctl service deploy --service my-service --image [tag]`
- Validates image exists in registry
- Patches Deployment in target namespace
- Waits for rollout, reports status
- Integrates with seed program for smoke tests

**Developer dashboard enhancements**
- "New service" button that guides through scaffold + provisioning
- Service quick-links: logs, metrics, traces, recent deployments
- Event subscriptions per service: filter/subscribe to NATS streams
- Cost estimate per service based on requests/limits

**Phase complete when**: Developer can `aeroctl service create --name taxi-booking`, get a working service in a namespace, and `aeroctl service deploy` it with zero platform team involvement.

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
- Weekends 5–6: flight board and ops dashboards (real-time web UIs)
- Weekends 7–10: full observability stack (metrics, logs, traces)
- Weekends 11–13: platform API, UI, aeroctl CLI
- Weekends 14–16: self-service layer (scaffold, provisioning, deployment workflow)
- Weekend 17+: stretch goals as interest develops

---

## Key Principles

- Build the seed program early and keep extending it — being able to fire `./hack/seed/seed --burst --flights 50` and watch everything light up in Grafana is what makes this feel real
- Don't rush to observability before the event chain is solid — Grafana graphs of nothing are not interesting
- `aeroctl` in Phase 4 directly transfers to `ppctl` — same cobra patterns, same CLI-calling-internal-API structure
- Keep the README good from day one — this is a portfolio piece
