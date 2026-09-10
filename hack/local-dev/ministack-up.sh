#!/usr/bin/env bash
# Start a local MiniStack container on the host, used as a fake AWS endpoint
# for the "run the manager on your machine" local dev flow.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

if docker inspect "${MINISTACK_CONTAINER_NAME}" >/dev/null 2>&1; then
	log "MiniStack container '${MINISTACK_CONTAINER_NAME}' already exists. Skipping creation."
else
	log "Starting MiniStack container '${MINISTACK_CONTAINER_NAME}' on port ${MINISTACK_HOST_PORT}..."
	docker run -d \
		--name "${MINISTACK_CONTAINER_NAME}" \
		-p "${MINISTACK_HOST_PORT}:4566" \
		"${MINISTACK_IMAGE}" >/dev/null
fi

log "Waiting for MiniStack to become healthy..."
for _ in $(seq 1 30); do
	if curl -fsS "http://localhost:${MINISTACK_HOST_PORT}/_ministack/health" >/dev/null 2>&1; then
		log "MiniStack is up at http://localhost:${MINISTACK_HOST_PORT}"
		exit 0
	fi
	sleep 1
done

log "ERROR: MiniStack did not become healthy in time."
docker logs "${MINISTACK_CONTAINER_NAME}" || true
exit 1
