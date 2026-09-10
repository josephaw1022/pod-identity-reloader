#!/usr/bin/env bash
# Full local deployment: create a Kind cluster, deploy MiniStack inside it,
# build the controller image locally and load it into Kind, seed MiniStack
# with a sample IAM role/EKS cluster/Pod Identity Association, and apply the
# operator manifests.
#
# This is the "kick the tires end-to-end" flow. For a lighter-weight loop
# that runs the manager as a local go process, use `make local-up` +
# `make local-run` instead.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

CONTROLLER_NAMESPACE="pod-identity-reloader-system"

"${SCRIPT_DIR}/kind-up.sh"

log "Deploying MiniStack into the Kind cluster..."
kubectl apply -k "${REPO_ROOT}/config/local-dev/ministack"
kubectl -n "${MINISTACK_NAMESPACE}" rollout status deployment/ministack --timeout=120s

log "Building the manager image '${MANAGER_IMAGE}'..."
make -C "${REPO_ROOT}" docker-build IMG="${MANAGER_IMAGE}"

log "Loading '${MANAGER_IMAGE}' into Kind cluster '${KIND_CLUSTER_NAME}'..."
kind load docker-image "${MANAGER_IMAGE}" --name "${KIND_CLUSTER_NAME}"

log "Deploying the operator manifests..."
kubectl apply -k "${REPO_ROOT}/config/local-dev/controller"

# The kustomize overlay bakes in the default LOCAL_DEV_CLUSTER_NAME. If the
# caller overrode it, force the deployed --cluster-name flag to match so it
# always agrees with whatever we seed into MiniStack below.
kubectl -n "${CONTROLLER_NAMESPACE}" patch deployment pod-identity-reloader-controller-manager \
	--type=json \
	-p="[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/args/3\",\"value\":\"--cluster-name=${LOCAL_DEV_CLUSTER_NAME}\"}]"

log "Waiting for MiniStack to be reachable from the host to seed AWS resources..."
kubectl -n "${MINISTACK_NAMESPACE}" port-forward svc/"${MINISTACK_SERVICE_NAME}" 4566:4566 >/tmp/pod-identity-reloader-ministack-pf.log 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 30); do
	if curl -fsS "http://localhost:4566/_ministack/health" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

"${SCRIPT_DIR}/seed-aws.sh"

kill "${PF_PID}" >/dev/null 2>&1 || true
trap - EXIT

log "Waiting for the controller to become ready..."
kubectl -n "${CONTROLLER_NAMESPACE}" rollout status deployment/pod-identity-reloader-controller-manager --timeout=120s

log "Local Kind deployment ready."
log "  kubectl --context kind-${KIND_CLUSTER_NAME} -n ${CONTROLLER_NAMESPACE} logs deploy/pod-identity-reloader-controller-manager -f"
