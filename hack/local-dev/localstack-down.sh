#!/usr/bin/env bash
# Stop and remove the local LocalStack container started by localstack-up.sh.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

if docker inspect "${LOCALSTACK_CONTAINER_NAME}" >/dev/null 2>&1; then
	log "Removing LocalStack container '${LOCALSTACK_CONTAINER_NAME}'..."
	docker rm -f "${LOCALSTACK_CONTAINER_NAME}" >/dev/null
else
	log "LocalStack container '${LOCALSTACK_CONTAINER_NAME}' not found. Nothing to do."
fi
