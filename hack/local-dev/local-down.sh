#!/usr/bin/env bash
# Tear down the docker-only local dev environment.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${SCRIPT_DIR}/localstack-down.sh"
