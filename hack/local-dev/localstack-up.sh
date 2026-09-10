#!/usr/bin/env bash
# Start a local LocalStack container on the host, used as a fake AWS
# endpoint (including EKS Pod Identity Associations) for local dev and e2e.
#
# LocalStack always runs as a plain Docker container on the host — never as
# an in-cluster Pod — because its EKS emulation spins up real k3d-backed
# clusters and needs access to the host's Docker socket. When used with the
# Kind flow (kind-deploy.sh), the controller reaches this container via the
# kind Docker network's gateway IP; see kind-deploy.sh and
# test/e2e/e2e_test.go.
#
# Requires a LocalStack Pro auth token: EKS Pod Identity emulation
# (CreatePodIdentityAssociation, etc.) is a Pro-only feature. See
# https://docs.localstack.cloud/getting-started/auth-token/ to obtain one.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

require_localstack_auth_token

if docker inspect "${LOCALSTACK_CONTAINER_NAME}" >/dev/null 2>&1; then
	log "LocalStack container '${LOCALSTACK_CONTAINER_NAME}' already exists. Skipping creation."
else
	log "Starting LocalStack container '${LOCALSTACK_CONTAINER_NAME}' on port ${LOCALSTACK_HOST_PORT}..."
	docker run -d \
		--name "${LOCALSTACK_CONTAINER_NAME}" \
		-p "${LOCALSTACK_HOST_PORT}:4566" \
		-e LOCALSTACK_AUTH_TOKEN="${LOCALSTACK_AUTH_TOKEN}" \
		-e SERVICES=eks,iam,sts \
		-v /var/run/docker.sock:/var/run/docker.sock \
		"${LOCALSTACK_IMAGE}" >/dev/null
fi

log "Waiting for LocalStack to become healthy..."
for _ in $(seq 1 60); do
	if curl -fsS "http://localhost:${LOCALSTACK_HOST_PORT}/_localstack/health" >/dev/null 2>&1; then
		log "LocalStack is up at http://localhost:${LOCALSTACK_HOST_PORT}"
		exit 0
	fi
	sleep 1
done

log "ERROR: LocalStack did not become healthy in time."
docker logs "${LOCALSTACK_CONTAINER_NAME}" || true
exit 1
