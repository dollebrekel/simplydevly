#!/usr/bin/env bash
# seed-marketplace-index.sh — Generates a sample marketplace-index.json
# at ~/.siply/cache/marketplace-index.json for local development.
#
# Usage:
#   ./scripts/seed-marketplace-index.sh
#   make marketplace-seed
#
# The generated file is a copy of the canonical fixture used by unit tests:
#   internal/marketplace/testdata/marketplace-index.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FIXTURE="${REPO_ROOT}/internal/marketplace/testdata/marketplace-index.json"

if [[ ! -f "${FIXTURE}" ]]; then
  echo "ERROR: fixture not found at ${FIXTURE}" >&2
  exit 1
fi

CACHE_DIR="${HOME}/.siply/cache"
mkdir -p "${CACHE_DIR}"

cp "${FIXTURE}" "${CACHE_DIR}/marketplace-index.json"
echo "✓ Seeded marketplace index at ${CACHE_DIR}/marketplace-index.json"
