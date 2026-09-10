#!/usr/bin/env bash
# Shared configuration for the local development tooling.
#
# This project uses LocalStack (https://www.localstack.cloud/), a local AWS
# emulator, as a stand-in for a real AWS account/EKS cluster during local
# development. Emulating EKS Pod Identity Associations (CreatePodIdentityAssociation,
# ListPodIdentityAssociations, DescribePodIdentityAssociation) requires
# LocalStack Pro — see LOCALSTACK_AUTH_TOKEN below. Source this file from
# other local-dev scripts; do not execute it directly.
set -euo pipefail

# Name shared by the "EKS cluster" seeded in LocalStack and the --cluster-name
# flag passed to the controller. These MUST match, or the controller will not
# find the Pod Identity Associations seeded below.
LOCAL_DEV_CLUSTER_NAME="${LOCAL_DEV_CLUSTER_NAME:-pod-identity-reloader-local}"

# Kind cluster used for the "full local deployment" flow.
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-${LOCAL_DEV_CLUSTER_NAME}}"

# Docker network kind attaches its node containers to. This is shared across
# all kind clusters on the host unless KIND_EXPERIMENTAL_DOCKER_NETWORK is
# set, and is what lets a pod running inside Kind reach the LocalStack
# container's published port via the network's gateway IP (see
# kind-deploy.sh and test/e2e/e2e_test.go).
KIND_DOCKER_NETWORK="${KIND_DOCKER_NETWORK:-kind}"

# LocalStack docker container settings. LocalStack always runs as a plain
# Docker container on the host (never as an in-cluster Pod), because its EKS
# emulation spins up real k3d-backed clusters and needs access to the host's
# Docker socket.
LOCALSTACK_IMAGE="${LOCALSTACK_IMAGE:-localstack/localstack:latest}"
LOCALSTACK_CONTAINER_NAME="${LOCALSTACK_CONTAINER_NAME:-pod-identity-reloader-localstack}"
LOCALSTACK_HOST_PORT="${LOCALSTACK_HOST_PORT:-4566}"

# Manager image built and loaded into Kind for the full local deployment.
# Must match the tag set by config/local-dev/controller/kustomization.yaml.
MANAGER_IMAGE="${MANAGER_IMAGE:-controller:local-dev}"

# LocalStack accepts any well-formed AWS credentials; these are local
# fixtures, not secrets, and are never used against real AWS.
AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-$AWS_REGION}"
LOCAL_DEV_AWS_ACCESS_KEY_ID="${LOCAL_DEV_AWS_ACCESS_KEY_ID:-test}"
LOCAL_DEV_AWS_SECRET_ACCESS_KEY="${LOCAL_DEV_AWS_SECRET_ACCESS_KEY:-test}"

# Sample workload identity seeded into LocalStack so the controller has
# something to reconcile against out of the box.
LOCAL_DEV_ROLE_NAME="${LOCAL_DEV_ROLE_NAME:-pod-identity-reloader-sample-role}"
LOCAL_DEV_NAMESPACE="${LOCAL_DEV_NAMESPACE:-default}"
LOCAL_DEV_SERVICE_ACCOUNT="${LOCAL_DEV_SERVICE_ACCOUNT:-pod-identity-reloader-sample}"

# Point the AWS CLI/SDK at LocalStack. LocalStack always publishes its
# gateway on the host, so this host-mapped address works for seeding scripts
# run directly on the host, regardless of whether the controller itself is
# running locally (go run) or deployed into Kind.
export AWS_ENDPOINT_URL="${AWS_ENDPOINT_URL:-http://localhost:${LOCALSTACK_HOST_PORT}}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-$LOCAL_DEV_AWS_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-$LOCAL_DEV_AWS_SECRET_ACCESS_KEY}"
export AWS_REGION

log() {
	echo "[local-dev] $*"
}

# require_localstack_auth_token fails fast with a helpful message if
# LOCALSTACK_AUTH_TOKEN is not set. EKS Pod Identity emulation is a
# LocalStack Pro feature and will not work without a valid token.
# See: https://docs.localstack.cloud/getting-started/auth-token/
require_localstack_auth_token() {
	if [ -z "${LOCALSTACK_AUTH_TOKEN:-}" ]; then
		echo "LOCALSTACK_AUTH_TOKEN is required (EKS Pod Identity emulation is a LocalStack Pro feature)." >&2
		echo "Get a token at https://app.localstack.cloud/ and export it: export LOCALSTACK_AUTH_TOKEN=..." >&2
		exit 1
	fi
}

# aws_local wraps the aws CLI and always passes --endpoint-url explicitly.
# Do not rely on the exported AWS_ENDPOINT_URL env var alone: some aws-cli
# versions/subcommands ignore it, so every call in this project's local-dev
# scripts must go through this wrapper to guarantee LocalStack (not real AWS)
# is targeted.
aws_local() {
	aws --endpoint-url "${AWS_ENDPOINT_URL}" "$@"
}
