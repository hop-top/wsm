#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_TAG="${IMAGE_TAG:-wsm-test-ubuntu:local}"

printf 'Building Ubuntu test image %s\n' "$IMAGE_TAG"
docker build -f "$ROOT_DIR/Dockerfile.ubuntu-test" -t "$IMAGE_TAG" "$ROOT_DIR"

printf 'Running full test matrix in Ubuntu container\n'
docker run --rm -t "$IMAGE_TAG"
