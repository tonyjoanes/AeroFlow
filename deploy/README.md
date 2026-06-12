# Running AeroFlow on `kind`

These steps stand up the full Phase 2 event chain: a `kind` cluster, NATS
JetStream, all seven services, NGINX ingress with TLS, and the seed program.

## 1. Create the cluster and namespaces

```bash
kind create cluster --name aeroflow --config deploy/cluster/kind-config.yaml
kubectl apply -f deploy/cluster/namespaces.yaml
```

## 2. Deploy NATS JetStream

```bash
kubectl apply -f deploy/nats/nats.yaml
kubectl -n platform rollout status deployment/nats
```

The `AEROFLOW` stream is created automatically by the services on connect —
no manual stream setup needed.

## 3. Install NGINX ingress controller and cert-manager

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add jetstack https://charts.jetstack.io
helm repo update

helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace platform \
  --set controller.service.type=NodePort \
  --set controller.service.nodePorts.http=30080 \
  --set controller.service.nodePorts.https=30443

helm install cert-manager jetstack/cert-manager \
  --namespace platform \
  --set crds.enabled=true

kubectl apply -f deploy/ingress/cluster-issuer.yaml
```

Add `aeroflow.local` to your hosts file so the ingress host header resolves:

**Windows** (`C:\Windows\System32\drivers\etc\hosts`):
```
127.0.0.1  aeroflow.local
```

**macOS/Linux** (`/etc/hosts`):
```
127.0.0.1  aeroflow.local
```

## 4. Build and load all service images

```bash
docker build -t aeroflow/flight-service:dev        -f flight-service/Dockerfile .
docker build -t aeroflow/gate-service:dev           -f gate-service/Dockerfile .
docker build -t aeroflow/baggage-service:dev        -f baggage-service/Dockerfile .
docker build -t aeroflow/carousel-service:dev       -f carousel-service/Dockerfile .
docker build -t aeroflow/turnaround-service:dev     -f turnaround-service/Dockerfile .
docker build -t aeroflow/crew-dispatch-service:dev  -f crew-dispatch-service/Dockerfile .
docker build -t aeroflow/notification-service:dev   -f notification-service/Dockerfile .

kind load docker-image aeroflow/flight-service:dev        --name aeroflow
kind load docker-image aeroflow/gate-service:dev           --name aeroflow
kind load docker-image aeroflow/baggage-service:dev        --name aeroflow
kind load docker-image aeroflow/carousel-service:dev       --name aeroflow
kind load docker-image aeroflow/turnaround-service:dev     --name aeroflow
kind load docker-image aeroflow/crew-dispatch-service:dev  --name aeroflow
kind load docker-image aeroflow/notification-service:dev   --name aeroflow
```

## 5. Deploy all services

```bash
kubectl apply -f deploy/flight-service/flight-service.yaml
kubectl apply -f deploy/gate-service/gate-service.yaml
kubectl apply -f deploy/baggage-service/baggage-service.yaml
kubectl apply -f deploy/carousel-service/carousel-service.yaml
kubectl apply -f deploy/turnaround-service/turnaround-service.yaml
kubectl apply -f deploy/crew-dispatch-service/crew-dispatch-service.yaml
kubectl apply -f deploy/notification-service/notification-service.yaml
kubectl apply -f deploy/ingress/ingress.yaml

kubectl -n flights      rollout status deployment/flight-service
kubectl -n gates        rollout status deployment/gate-service
kubectl -n baggage      rollout status deployment/baggage-service
kubectl -n baggage      rollout status deployment/carousel-service
kubectl -n ground-ops   rollout status deployment/turnaround-service
kubectl -n ground-ops   rollout status deployment/crew-dispatch-service
kubectl -n notifications rollout status deployment/notification-service
```

## 6. Trigger the event chain

**Single flight via curl** (NodePort, bypassing ingress):
```bash
curl -X POST localhost:8080/flights/land \
  -H "Content-Type: application/json" \
  -d '{"number":"BA442","origin":"LHR","destination":"JFK"}'
```

**Single flight via ingress** (TLS — `-k` skips self-signed cert warning):
```bash
curl -k -X POST https://aeroflow.local:30443/api/flights/land \
  -H "Content-Type: application/json" \
  -d '{"number":"BA442","origin":"LHR","destination":"JFK"}'
```

**Burst 50 flights with the seed program** (great for watching Grafana):
```bash
go run ./hack/seed --addr http://localhost:8080 --flights 50 --burst
```

**Steady trickle at 2-second intervals:**
```bash
go run ./hack/seed --addr http://localhost:8080 --flights 20 --interval 2s
```

## 7. Watch the event chain

```bash
kubectl -n flights       logs -l app=flight-service        -f
kubectl -n gates         logs -l app=gate-service          -f
kubectl -n baggage       logs -l app=baggage-service       -f
kubectl -n baggage       logs -l app=carousel-service      -f
kubectl -n ground-ops    logs -l app=turnaround-service    -f
kubectl -n ground-ops    logs -l app=crew-dispatch-service -f
kubectl -n notifications logs -l app=notification-service  -f
```

Every log line across all services will share the same `correlation_id` for
a given flight — that's the full chain traceable from a single landing.

## 8. Tear down

```bash
kind delete cluster --name aeroflow
```
