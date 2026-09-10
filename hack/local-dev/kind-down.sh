#!/usr/bin/env bash
# Delete the local Kind cluster created by kind-up.sh (or kind-deploy.sh).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

if kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER_NAME}"; then
	log "Deleting Kind cluster '${KIND_CLUSTER_NAME}'..."
	kind delete cluster --name "${KIND_CLUSTER_NAME}"
else
	log "Kind cluster '${KIND_CLUSTER_NAME}' not found. Nothing to do."
fi
