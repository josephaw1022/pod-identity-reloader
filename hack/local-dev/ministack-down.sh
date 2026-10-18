#!/usr/bin/env bash
# Stop and remove the local MiniStack container started by ministack-up.sh.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

if docker inspect "${MINISTACK_CONTAINER_NAME}" >/dev/null 2>&1; then
	log "Removing MiniStack container '${MINISTACK_CONTAINER_NAME}'..."
	docker rm -f "${MINISTACK_CONTAINER_NAME}" >/dev/null
else
	log "MiniStack container '${MINISTACK_CONTAINER_NAME}' not found. Nothing to do."
fi
