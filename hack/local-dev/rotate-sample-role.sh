#!/usr/bin/env bash
# Rotate the IAM role bound to the sample ServiceAccount's Pod Identity
# Association in LocalStack. Lets local dev/e2e tests exercise the
# controller's role-change detection without touching real AWS.
#
# Usage: rotate-sample-role.sh [new-role-name]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

NEW_ROLE_NAME="${1:-${LOCAL_DEV_ROLE_NAME}-rotated}"

command -v aws >/dev/null 2>&1 || {
	echo "aws CLI is required. Install it: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html" >&2
	exit 1
}

TRUST_POLICY=$(cat <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Service": "pods.eks.amazonaws.com" },
      "Action": ["sts:AssumeRole", "sts:TagSession"]
    }
  ]
}
JSON
)

NEW_ROLE_ARN=$(aws_local iam get-role --role-name "${NEW_ROLE_NAME}" \
	--query 'Role.Arn' --output text 2>/dev/null || true)

if [ -z "${NEW_ROLE_ARN}" ] || [ "${NEW_ROLE_ARN}" = "None" ]; then
	log "Creating rotated IAM role '${NEW_ROLE_NAME}'..."
	NEW_ROLE_ARN=$(aws_local iam create-role \
		--role-name "${NEW_ROLE_NAME}" \
		--assume-role-policy-document "${TRUST_POLICY}" \
		--query 'Role.Arn' --output text)
fi

ASSOC_ID=$(aws_local eks list-pod-identity-associations \
	--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
	--namespace "${LOCAL_DEV_NAMESPACE}" \
	--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
	--query 'associations[0].associationId' --output text 2>/dev/null || true)

if [ -z "${ASSOC_ID}" ] || [ "${ASSOC_ID}" = "None" ]; then
	echo "No existing Pod Identity Association found for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}. Run seed-aws.sh first." >&2
	exit 1
fi

log "Updating Pod Identity Association '${ASSOC_ID}' to role '${NEW_ROLE_NAME}'..."
aws_local eks update-pod-identity-association \
	--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
	--association-id "${ASSOC_ID}" \
	--role-arn "${NEW_ROLE_ARN}" \
	>/dev/null

log "Rotated role ARN: ${NEW_ROLE_ARN}"
