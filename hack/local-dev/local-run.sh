#!/usr/bin/env bash
# Run the manager locally (go run) against the MiniStack container started by
# local-up.sh.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

log "Running manager locally with cluster-name=${LOCAL_DEV_CLUSTER_NAME} against ${AWS_ENDPOINT_URL}"
exec go run "${REPO_ROOT}/cmd/main.go" \
	--cluster-name="${LOCAL_DEV_CLUSTER_NAME}" \
	--metrics-bind-address=:8080 \
	--metrics-secure=false \
	"$@"
