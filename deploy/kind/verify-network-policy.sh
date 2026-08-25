#!/bin/sh

set -eu

context=${1:?Kubernetes context is required}
image=${2:?probe image is required}
namespace=threadline-dev
allow_pod=threadline-netpol-allow-probe
otel_allow_pod=threadline-netpol-otel-allow-probe
worker_allow_pod=threadline-netpol-worker-allow-probe
deny_pod=threadline-netpol-deny-probe

cleanup() {
  kubectl --context "$context" --namespace "$namespace" delete pod \
    --selector=threadline.io/network-policy-probe=true \
    --ignore-not-found --wait >/dev/null 2>&1 || true
}

trap cleanup EXIT
trap 'exit 130' HUP INT TERM
cleanup

otel_ip=$(kubectl --context "$context" --namespace "$namespace" get pods \
  --selector=app=otel-collector --field-selector=status.phase=Running \
  --output=jsonpath='{.items[0].status.podIP}')
jaeger_ip=$(kubectl --context "$context" --namespace "$namespace" get pods \
  --selector=app=jaeger --field-selector=status.phase=Running \
  --output=jsonpath='{.items[0].status.podIP}')
nats_ip=$(kubectl --context "$context" --namespace "$namespace" get pods \
  --selector=app=nats --field-selector=status.phase=Running \
  --output=jsonpath='{.items[0].status.podIP}')
[ -n "$otel_ip" ] && [ -n "$jaeger_ip" ] && [ -n "$nats_ip" ] || {
  echo "NetworkPolicy probe targets are not running" >&2
  exit 1
}

kubectl --context "$context" apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: $allow_pod
  namespace: $namespace
  labels:
    app: prometheus
    threadline.io/network-policy-probe: "true"
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  readinessGates:
    - conditionType: threadline.io/network-policy-probe-ready
  containers:
    - name: probe
      image: $image
      imagePullPolicy: Never
      command: [sh, -c, "sleep 3600"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
---
apiVersion: v1
kind: Pod
metadata:
  name: $otel_allow_pod
  namespace: $namespace
  labels:
    app: otel-collector
    threadline.io/network-policy-probe: "true"
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  readinessGates:
    - conditionType: threadline.io/network-policy-probe-ready
  containers:
    - name: probe
      image: $image
      imagePullPolicy: Never
      command: [sh, -c, "sleep 3600"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
---
apiVersion: v1
kind: Pod
metadata:
  name: $worker_allow_pod
  namespace: $namespace
  labels:
    threadline.io/nats-principal: worker
    threadline.io/network-policy-probe: "true"
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  containers:
    - name: probe
      image: $image
      imagePullPolicy: Never
      command: [sh, -c, "sleep 3600"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
---
apiVersion: v1
kind: Pod
metadata:
  name: $deny_pod
  namespace: $namespace
  labels:
    app: threadline-netpol-denied-probe
    threadline.io/network-policy-probe: "true"
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  containers:
    - name: probe
      image: $image
      imagePullPolicy: Never
      command: [sh, -c, "sleep 3600"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
EOF

kubectl --context "$context" --namespace "$namespace" wait \
  --for=jsonpath='{.status.phase}'=Running \
  "pod/$allow_pod" "pod/$otel_allow_pod" "pod/$worker_allow_pod" "pod/$deny_pod" --timeout=90s

# NetworkPolicy controllers reconcile asynchronously. Require the permitted
# paths, DNS for the denied source, and all denied paths to agree repeatedly
# before passing. The allow Pods' readiness gates keep them out of the
# Prometheus and OTel Service endpoints even though they carry the labels
# selected by those Services and policies.
deadline=$(( $(date +%s) + 60 ))
stable=0
while [ "$(date +%s)" -lt "$deadline" ]; do
  if kubectl --context "$context" --namespace "$namespace" exec "$allow_pod" -- \
      wget -T 3 -q -O /dev/null "http://$otel_ip:8889/metrics" >/dev/null 2>&1 && \
    kubectl --context "$context" --namespace "$namespace" exec "$otel_allow_pod" -- \
      nc -z -w 2 "$jaeger_ip" 4317 >/dev/null 2>&1 && \
    kubectl --context "$context" --namespace "$namespace" exec "$worker_allow_pod" -- \
      nc -z -w 2 "$nats_ip" 4222 >/dev/null 2>&1 && \
    kubectl --context "$context" --namespace "$namespace" exec "$deny_pod" -- \
      nslookup otel-collector.threadline-dev.svc.cluster.local >/dev/null 2>&1 && \
    kubectl --context "$context" --namespace "$namespace" exec "$deny_pod" -- \
      nslookup jaeger.threadline-dev.svc.cluster.local >/dev/null 2>&1 && \
    ! kubectl --context "$context" --namespace "$namespace" exec "$deny_pod" -- \
      nc -z -w 2 "$otel_ip" 8889 >/dev/null 2>&1 && \
    ! kubectl --context "$context" --namespace "$namespace" exec "$deny_pod" -- \
      nc -z -w 2 "$jaeger_ip" 4317 >/dev/null 2>&1 && \
    ! kubectl --context "$context" --namespace "$namespace" exec "$deny_pod" -- \
      nc -z -w 2 "$nats_ip" 4222 >/dev/null 2>&1; then
    stable=$((stable + 1))
    if [ "$stable" -ge 3 ]; then
      echo "NetworkPolicy probes passed: prometheus -> otel:8889, otel -> jaeger:4317, and worker -> nats:4222 allowed; unlabeled source denied"
      exit 0
    fi
  else
    stable=0
  fi
  sleep 2
done

echo "NetworkPolicy probes did not converge within 60s" >&2
for check in \
  "prometheus-to-otel:wget -T 3 -q -O /dev/null http://$otel_ip:8889/metrics" \
  "otel-to-jaeger:nc -z -w 2 $jaeger_ip 4317" \
  "worker-to-nats:nc -z -w 2 $nats_ip 4222" \
  "deny-dns-otel:nslookup otel-collector.threadline-dev.svc.cluster.local" \
  "deny-dns-jaeger:nslookup jaeger.threadline-dev.svc.cluster.local" \
  "deny-to-otel:nc -z -w 2 $otel_ip 8889" \
  "deny-to-jaeger:nc -z -w 2 $jaeger_ip 4317" \
  "deny-to-nats:nc -z -w 2 $nats_ip 4222"; do
  name=${check%%:*}
  command=${check#*:}
  pod=$deny_pod
  case "$name" in
    prometheus-to-otel) pod=$allow_pod ;;
    otel-to-jaeger) pod=$otel_allow_pod ;;
    worker-to-nats) pod=$worker_allow_pod ;;
  esac
  if kubectl --context "$context" --namespace "$namespace" exec "$pod" -- sh -c "$command" >/dev/null 2>&1; then
    echo "$name: connected" >&2
  else
    echo "$name: blocked-or-failed" >&2
  fi
done
exit 1
