#!/usr/bin/env bash
# Full local deployment: create a Kind cluster, start LocalStack on the host,
# build the controller image locally and load it into Kind, seed LocalStack
# with a sample IAM role/EKS cluster/Pod Identity Association, and apply the
# operator manifests pointed at LocalStack.
#
# This is the "kick the tires end-to-end" flow. For a lighter-weight loop
# that runs the manager as a local go process, use `make local-up` +
# `make local-run` instead.
#
# Requires a LocalStack Pro auth token; see hack/local-dev/localstack-up.sh.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

CONTROLLER_NAMESPACE="pod-identity-reloader-system"

# kind-up.sh must run before localstack-up.sh: it creates the "kind" Docker
# network (see KIND_DOCKER_NETWORK in env.sh) whose gateway IP we use below
# to let pods inside Kind reach the host-level LocalStack container.
"${SCRIPT_DIR}/kind-up.sh"
"${SCRIPT_DIR}/localstack-up.sh"

log "Building the manager image '${MANAGER_IMAGE}'..."
make -C "${REPO_ROOT}" docker-build IMG="${MANAGER_IMAGE}"

log "Loading '${MANAGER_IMAGE}' into Kind cluster '${KIND_CLUSTER_NAME}'..."
kind load docker-image "${MANAGER_IMAGE}" --name "${KIND_CLUSTER_NAME}"

log "Deploying the operator manifests..."
kubectl apply -k "${REPO_ROOT}/config/local-dev/controller"

# LocalStack is a host-level Docker container, not an in-cluster Service, so
# the AWS_ENDPOINT_URL baked into manager_local_dev_patch.yaml is only a
# placeholder. Resolve the kind Docker network's gateway IP (reachable from
# pods running inside Kind, since Docker publishes the container's port on
# every host interface, including that gateway) and patch it in, along with
# --cluster-name in case the caller overrode LOCAL_DEV_CLUSTER_NAME.
KIND_GATEWAY_IP="$(docker network inspect "${KIND_DOCKER_NETWORK}" \
	--format '{{ (index .IPAM.Config 0).Gateway }}')"
LOCALSTACK_KIND_ENDPOINT="http://${KIND_GATEWAY_IP}:${LOCALSTACK_HOST_PORT}"

log "Pointing the controller-manager at LocalStack (${LOCALSTACK_KIND_ENDPOINT})..."
kubectl -n "${CONTROLLER_NAMESPACE}" set env deployment/pod-identity-reloader-controller-manager \
	"AWS_ENDPOINT_URL=${LOCALSTACK_KIND_ENDPOINT}"

kubectl -n "${CONTROLLER_NAMESPACE}" patch deployment pod-identity-reloader-controller-manager \
	--type=json \
	-p="[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/args/3\",\"value\":\"--cluster-name=${LOCAL_DEV_CLUSTER_NAME}\"}]"

log "Seeding LocalStack with a sample IAM role, EKS cluster, and Pod Identity Association..."
"${SCRIPT_DIR}/seed-aws.sh"

log "Waiting for the controller to become ready..."
kubectl -n "${CONTROLLER_NAMESPACE}" rollout status deployment/pod-identity-reloader-controller-manager --timeout=120s

log "Local Kind deployment ready."
log "  kubectl --context kind-${KIND_CLUSTER_NAME} -n ${CONTROLLER_NAMESPACE} logs deploy/pod-identity-reloader-controller-manager -f"
