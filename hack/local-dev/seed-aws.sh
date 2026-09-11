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
	# EKS's CreateCluster validates its --resources-vpc-config subnet IDs
	# against real EC2 resources (via DescribeSubnets), so a VPC and
	# subnets must actually exist in LocalStack before the cluster does.
	VPC_ID=$(aws_local ec2 describe-vpcs \
		--filters "Name=tag:Name,Values=${LOCAL_DEV_CLUSTER_NAME}" \
		--query 'Vpcs[0].VpcId' --output text 2>/dev/null || true)

	if [ -z "${VPC_ID}" ] || [ "${VPC_ID}" = "None" ]; then
		log "Creating VPC for '${LOCAL_DEV_CLUSTER_NAME}'..."
		VPC_ID=$(aws_local ec2 create-vpc --cidr-block 10.0.0.0/16 \
			--query 'Vpc.VpcId' --output text)
		aws_local ec2 create-tags --resources "${VPC_ID}" \
			--tags "Key=Name,Value=${LOCAL_DEV_CLUSTER_NAME}" >/dev/null
	else
		log "VPC for '${LOCAL_DEV_CLUSTER_NAME}' already exists."
	fi

	SUBNET_IDS=$(aws_local ec2 describe-subnets \
		--filters "Name=vpc-id,Values=${VPC_ID}" \
		--query 'Subnets[].SubnetId' --output text 2>/dev/null || true)

	if [ -z "${SUBNET_IDS}" ]; then
		log "Creating subnets for '${LOCAL_DEV_CLUSTER_NAME}'..."
		SUBNET_ID_1=$(aws_local ec2 create-subnet --vpc-id "${VPC_ID}" \
			--cidr-block 10.0.1.0/24 --availability-zone "${AWS_REGION}a" \
			--query 'Subnet.SubnetId' --output text)
		SUBNET_ID_2=$(aws_local ec2 create-subnet --vpc-id "${VPC_ID}" \
			--cidr-block 10.0.2.0/24 --availability-zone "${AWS_REGION}b" \
			--query 'Subnet.SubnetId' --output text)
		SUBNET_IDS="${SUBNET_ID_1} ${SUBNET_ID_2}"
	else
		log "Subnets for '${LOCAL_DEV_CLUSTER_NAME}' already exist."
	fi

	log "Creating EKS cluster '${LOCAL_DEV_CLUSTER_NAME}'..."
	aws_local eks create-cluster \
		--name "${LOCAL_DEV_CLUSTER_NAME}" \
		--role-arn "${ROLE_ARN}" \
		--resources-vpc-config "subnetIds=$(echo "${SUBNET_IDS}" | tr ' ' ',')" \
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
	CREATE_ASSOC_OUTPUT=""
	if ! CREATE_ASSOC_OUTPUT=$(aws_local eks create-pod-identity-association \
		--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
		--namespace "${LOCAL_DEV_NAMESPACE}" \
		--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
		--role-arn "${ROLE_ARN}" 2>&1); then
		if ! echo "${CREATE_ASSOC_OUTPUT}" | grep -q "ResourceInUseException"; then
			echo "${CREATE_ASSOC_OUTPUT}" >&2
			exit 1
		fi

		# LocalStack can report a service account as already associated
		# even though our own list check above found nothing (its EKS
		# Pod Identity emulation is backed by a separate k3d-backed
		# cluster, so association visibility can lag cluster/list calls).
		# Recover by finding and replacing whatever association actually
		# exists for this namespace/service account.
		log "Pod Identity Association for ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT} already exists; reconciling it..."
		CONFLICTING_ASSOC=$(aws_local eks list-pod-identity-associations \
			--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
			--namespace "${LOCAL_DEV_NAMESPACE}" \
			--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
			--query 'associations[0].associationId' --output text 2>/dev/null || true)

		if [ -z "${CONFLICTING_ASSOC}" ] || [ "${CONFLICTING_ASSOC}" = "None" ]; then
			log "ERROR: could not find the conflicting Pod Identity Association to reconcile."
			echo "${CREATE_ASSOC_OUTPUT}" >&2
			exit 1
		fi

		log "Deleting conflicting Pod Identity Association '${CONFLICTING_ASSOC}'..."
		aws_local eks delete-pod-identity-association \
			--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
			--association-id "${CONFLICTING_ASSOC}" >/dev/null

		aws_local eks create-pod-identity-association \
			--cluster-name "${LOCAL_DEV_CLUSTER_NAME}" \
			--namespace "${LOCAL_DEV_NAMESPACE}" \
			--service-account "${LOCAL_DEV_SERVICE_ACCOUNT}" \
			--role-arn "${ROLE_ARN}" \
			>/dev/null
	fi
fi

log "Seeding complete:"
log "  cluster-name:    ${LOCAL_DEV_CLUSTER_NAME}"
log "  role-arn:        ${ROLE_ARN}"
log "  namespace/sa:    ${LOCAL_DEV_NAMESPACE}/${LOCAL_DEV_SERVICE_ACCOUNT}"
