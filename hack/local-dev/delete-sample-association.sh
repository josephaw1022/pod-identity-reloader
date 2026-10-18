#!/usr/bin/env bash
# Delete the Pod Identity Association bound to the sample ServiceAccount in
# MiniStack. Lets local dev/e2e tests exercise the controller's
# association-removal detection without touching real AWS.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

command -v aws >/dev/null 2>&1 || {
	echo "aws CLI is required. Install it: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html" >&2
	exit 1
}

ASSOC_ID=$(aws_local eks list-pod-identity-associations \
	--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
	--namespace "${LOCAL_DEV_NAMESPACE}" \
	--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
	--query 'associations[0].associationId' --output text 2>/dev/null || true)

if [ -z "${ASSOC_ID}" ] || [ "${ASSOC_ID}" = "None" ]; then
	echo "No existing Pod Identity Association found for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}." >&2
	exit 1
fi

log "Deleting Pod Identity Association '${ASSOC_ID}' for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}..."
aws_local eks delete-pod-identity-association \
	--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
	--association-id "${ASSOC_ID}" \
	>/dev/null

log "Deleted Pod Identity Association '${ASSOC_ID}'"
