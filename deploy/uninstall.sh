#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

kubectl delete -f "${SCRIPT_DIR}/install.yaml" --ignore-not-found=true
