#!/usr/bin/env bash
# Runs the given command with AWS_ENDPOINT_URL pointed at a temporary
# kubectl port-forward to the in-cluster MiniStack service, then tears the
# port-forward down. Used to seed/rotate AWS resources in MiniStack from the
# host when MiniStack is running inside Kind (rather than as a local docker
# container).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

if [ "$#" -eq 0 ]; then
	echo "usage: $0 <command> [args...]" >&2
	exit 1
fi

kubectl -n "${MINISTACK_NAMESPACE}" port-forward "svc/${MINISTACK_SERVICE_NAME}" 4566:4566 \
	>/tmp/pod-identity-reloader-ministack-pf.log 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" >/dev/null 2>&1 || true' EXIT

log "Waiting for the port-forward to MiniStack to become ready..."
for _ in $(seq 1 30); do
	if curl -fsS "http://localhost:4566/_ministack/health" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

AWS_ENDPOINT_URL="http://localhost:4566" "$@"
