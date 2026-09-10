#!/usr/bin/env bash
# Seed LocalStack with the IAM role, EKS cluster, and Pod Identity Association
# the controller expects to find.
#
# Targets whatever AWS_ENDPOINT_URL is currently set to (see env.sh). This is
# always a host-reachable address, since LocalStack runs as a plain Docker
# container on the host for both the docker-only flow and the Kind flow (the
# controller running inside Kind reaches it separately via the kind Docker
# network's gateway IP; see kind-deploy.sh).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"

command -v aws >/dev/null 2>&1 || {
	echo "aws CLI is required. Install it: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html" >&2
	exit 1
}

log "Seeding LocalStack (${AWS_ENDPOINT_URL}) with cluster '${LOCAL_DEV_CLUSTER_NAME}'..."

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

ROLE_ARN=$(aws_local iam get-role --role-name "${LOCAL_DEV_ROLE_NAME}" \
	--query 'Role.Arn' --output text 2>/dev/null || true)

if [ -z "${ROLE_ARN}" ] || [ "${ROLE_ARN}" = "None" ]; then
	log "Creating IAM role '${LOCAL_DEV_ROLE_NAME}'..."
	ROLE_ARN=$(aws_local iam create-role \
		--role-name "${LOCAL_DEV_ROLE_NAME}" \
		--assume-role-policy-document "${TRUST_POLICY}" \
		--query 'Role.Arn' --output text)
else
	log "IAM role '${LOCAL_DEV_ROLE_NAME}' already exists."
fi

if aws_local eks describe-cluster --name "${LOCAL_DEV_CLUSTER_NAME}" >/dev/null 2>&1; then
	log "EKS cluster '${LOCAL_DEV_CLUSTER_NAME}' already exists."
else
	log "Creating EKS cluster '${LOCAL_DEV_CLUSTER_NAME}'..."
	aws_local eks create-cluster \
		--name "${LOCAL_DEV_CLUSTER_NAME}" \
		--role-arn "${ROLE_ARN}" \
		--resources-vpc-config subnetIds=subnet-local1,subnet-local2 \
		>/dev/null
fi

EXISTING_ASSOC=$(aws_local eks list-pod-identity-associations \
	--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
	--namespace "${LOCAL_DEV_NAMESPACE}" \
	--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
	--query 'associations[0].associationId' --output text 2>/dev/null || true)

if [ -n "${EXISTING_ASSOC}" ] && [ "${EXISTING_ASSOC}" != "None" ]; then
	log "Pod Identity Association for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT} already exists."
else
	log "Creating Pod Identity Association for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}..."
	aws_local eks create-pod-identity-association \
		--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
		--namespace "${LOCAL_DEV_NAMESPACE}" \
		--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
		--role-arn "${ROLE_ARN}" \
		>/dev/null
fi

log "Seeding complete:"
log "  cluster-name:    ${LOCAL_DEV_CLUSTER_NAME}"
log "  role-arn:        ${ROLE_ARN}"
log "  namespace/sa:    ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}"
