#!/usr/bin/env bash
# Packages the three Grafana dashboard JSON files into a ConfigMap and applies
# it to the cluster. The kube-prometheus-stack sidecar watches for ConfigMaps
# labelled grafana_dashboard=1 and loads them automatically — no manual import.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

kubectl create configmap aeroflow-dashboards \
  --namespace platform \
  --from-file="$DIR/dashboards/event-chain.json" \
  --from-file="$DIR/dashboards/service-detail.json" \
  --from-file="$DIR/dashboards/trace-explorer.json" \
  --dry-run=client -o yaml \
| kubectl label --local -f - grafana_dashboard=1 -o yaml \
| kubectl apply -f -

echo "Dashboards applied. Grafana will pick them up within ~30s."
