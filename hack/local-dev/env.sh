#!/usr/bin/env bash
# Shared configuration for the local development tooling.
#
# This project uses MiniStack (https://github.com/ministackorg/ministack), a
# free, open-source AWS emulator, as a stand-in for a real AWS account/EKS
# cluster during local development. Source this file from other local-dev
# scripts; do not execute it directly.
set -euo pipefail

# Name shared by the "EKS cluster" seeded in MiniStack and the --cluster-name
# flag passed to the controller. These MUST match, or the controller will not
# find the Pod Identity Associations seeded below.
LOCAL_DEV_CLUSTER_NAME="${LOCAL_DEV_CLUSTER_NAME:-pod-identity-reloader-local}"

# Kind cluster used for the "full local deployment" flow.
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-${LOCAL_DEV_CLUSTER_NAME}}"

# MiniStack docker container settings (used by the docker-only flow, i.e.
# `make local-up` + `make run`).
MINISTACK_IMAGE="${MINISTACK_IMAGE:-ministackorg/ministack:latest}"
MINISTACK_CONTAINER_NAME="${MINISTACK_CONTAINER_NAME:-pod-identity-reloader-ministack}"
MINISTACK_HOST_PORT="${MINISTACK_HOST_PORT:-4566}"

# MiniStack-in-kind settings (used by the `make local-kind-deploy` flow).
MINISTACK_NAMESPACE="${MINISTACK_NAMESPACE:-ministack-system}"
MINISTACK_SERVICE_NAME="${MINISTACK_SERVICE_NAME:-ministack}"
# In-cluster DNS name the controller uses to reach MiniStack when deployed to Kind.
MINISTACK_IN_CLUSTER_ENDPOINT="http://${MINISTACK_SERVICE_NAME}.${MINISTACK_NAMESPACE}.svc.cluster.local:4566"

# Manager image built and loaded into Kind for the full local deployment.
# Must match the tag set by config/local-dev/controller/kustomization.yaml.
MANAGER_IMAGE="${MANAGER_IMAGE:-controller:local-dev}"

# MiniStack accepts any well-formed AWS credentials; these are local fixtures,
# not secrets, and are never used against real AWS.
AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-$AWS_REGION}"
LOCAL_DEV_AWS_ACCESS_KEY_ID="${LOCAL_DEV_AWS_ACCESS_KEY_ID:-000000000000}"
LOCAL_DEV_AWS_SECRET_ACCESS_KEY="${LOCAL_DEV_AWS_SECRET_ACCESS_KEY:-ministack}"

# Sample workload identity seeded into MiniStack so the controller has
# something to reconcile against out of the box.
LOCAL_DEV_ROLE_NAME="${LOCAL_DEV_ROLE_NAME:-pod-identity-reloader-sample-role}"
LOCAL_DEV_NAMESPACE="${LOCAL_DEV_NAMESPACE:-default}"
LOCAL_DEV_SERVICE_ACCOUNT="${LOCAL_DEV_SERVICE_ACCOUNT:-pod-identity-reloader-sample}"

# Point the AWS CLI/SDK at MiniStack. Callers may override AWS_ENDPOINT_URL
# before sourcing this file (e.g. to target the in-cluster service instead of
# the host-mapped port).
export AWS_ENDPOINT_URL="${AWS_ENDPOINT_URL:-http://localhost:${MINISTACK_HOST_PORT}}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-$LOCAL_DEV_AWS_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-$LOCAL_DEV_AWS_SECRET_ACCESS_KEY}"
export AWS_REGION

log() {
	echo "[local-dev] $*"
}

# aws_local wraps the aws CLI and always passes --endpoint-url explicitly.
# Do not rely on the exported AWS_ENDPOINT_URL env var alone: some aws-cli
# versions/subcommands ignore it, so every call in this project's local-dev
# scripts must go through this wrapper to guarantee MiniStack (not real AWS)
# is targeted.
aws_local() {
	aws --endpoint-url "${AWS_ENDPOINT_URL}" "$@"
}
