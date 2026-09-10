#!/usr/bin/env bash
# Create the local Kind cluster used for the full local deployment, if it
# does not already exist.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

command -v kind >/dev/null 2>&1 || {
	echo "kind is required: https://kind.sigs.k8s.io/" >&2
	exit 1
}

if kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER_NAME}"; then
	log "Kind cluster '${KIND_CLUSTER_NAME}' already exists. Skipping creation."
else
	log "Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
	kind create cluster --name "${KIND_CLUSTER_NAME}"
fi

kubectl config use-context "kind-${KIND_CLUSTER_NAME}" >/dev/null
