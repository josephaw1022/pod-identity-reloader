#!/usr/bin/env bash
# Bring up the docker-only local dev environment: start LocalStack and seed
# it with a sample IAM role, EKS cluster, and Pod Identity Association.
#
# Requires a LocalStack Pro auth token; see hack/local-dev/localstack-up.sh.
#
# After this completes, run the manager locally against it with:
#   make local-run
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/localstack-up.sh"
"${SCRIPT_DIR}/seed-aws.sh"

# shellcheck source=hack/local-dev/env.sh
source "${SCRIPT_DIR}/env.sh"
log "Local LocalStack environment ready. Start the manager with: make local-run"
