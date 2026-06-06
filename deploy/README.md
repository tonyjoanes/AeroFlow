# Running AeroFlow on `kind`

These steps stand up the Phase 1 skeleton: a `kind` cluster, NATS JetStream,
and `flight-service`/`gate-service` exchanging real events.

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

The `AEROFLOW` stream is created automatically by the Go services on connect
(see `internal/messaging`), so there's no separate stream-setup step.

## 3. Build and load the service images

`kind` doesn't pull from a registry by default, so build locally and load the
images straight into the cluster's nodes:

```bash
docker build -t aeroflow/flight-service:dev -f flight-service/Dockerfile .
docker build -t aeroflow/gate-service:dev   -f gate-service/Dockerfile .

kind load docker-image aeroflow/flight-service:dev --name aeroflow
kind load docker-image aeroflow/gate-service:dev   --name aeroflow
```

## 4. Deploy the services

```bash
kubectl apply -f deploy/flight-service/flight-service.yaml
kubectl apply -f deploy/gate-service/gate-service.yaml

kubectl -n flights rollout status deployment/flight-service
kubectl -n gates   rollout status deployment/gate-service
```

## 5. Trigger the event chain

The kind config maps the cluster's NodePort 30080 to `localhost:8080`:

```bash
curl -X POST localhost:8080/flights/land \
  -H "Content-Type: application/json" \
  -d '{"number":"BA442","origin":"LHR","destination":"JFK"}'
```

Then check the logs:

```bash
kubectl -n flights logs -l app=flight-service --tail=20
kubectl -n gates   logs -l app=gate-service   --tail=20
```

You should see the same flow as running locally with `go run`: flight-service
publishes a `LANDED` event, gate-service consumes it and assigns a gate, and
the `correlation_id` matches across both log lines.

## 6. Tear down

```bash
kind delete cluster --name aeroflow
```
