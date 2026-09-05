#!/usr/bin/env bash
#
# Build the HeapBuddy image locally and run it.
#
# "Pushes locally" means the image is loaded into your local Docker daemon (a
# plain `docker build`); set REGISTRY=... to also `docker push` it to a registry.
#
# Usage:
#   scripts/docker-local.sh                 # build heapbuddy:local and run on :8080
#   PORT=9000 scripts/docker-local.sh       # run on a different host port
#   TAG=dev scripts/docker-local.sh         # build/run heapbuddy:dev
#   REGISTRY=ghcr.io/sachin-handiekar scripts/docker-local.sh   # also push
#
# Env vars (all optional):
#   IMAGE      image name                 (default: heapbuddy)
#   TAG        image tag                  (default: local)
#   NAME       container name             (default: heapbuddy)
#   PORT       host port -> 8080          (default: 8080)
#   REGISTRY   if set, retag + docker push <REGISTRY>/<IMAGE>:<TAG>

set -euo pipefail

# Always operate from the repo root (this script lives in scripts/).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

IMAGE="${IMAGE:-heapbuddy}"
TAG="${TAG:-local}"
NAME="${NAME:-heapbuddy}"
PORT="${PORT:-8080}"
REF="${IMAGE}:${TAG}"

# Version metadata baked into the binary via ldflags (see Dockerfile ARGs).
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> Building ${REF} (version=${VERSION} commit=${COMMIT})"
docker build \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  --build-arg DATE="${DATE}" \
  -t "${REF}" \
  .

if [ -n "${REGISTRY:-}" ]; then
  REMOTE="${REGISTRY}/${IMAGE}:${TAG}"
  echo "==> Pushing ${REMOTE}"
  docker tag "${REF}" "${REMOTE}"
  docker push "${REMOTE}"
fi

echo "==> Restarting container ${NAME} on host port ${PORT}"
docker rm -f "${NAME}" >/dev/null 2>&1 || true
docker run -d \
  --name "${NAME}" \
  --restart unless-stopped \
  -p "${PORT}:8080" \
  "${REF}" >/dev/null

# Wait for the server to answer its health probe.
echo -n "==> Waiting for http://localhost:${PORT}/healthz "
for _ in $(seq 1 30); do
  if curl -fsS "http://localhost:${PORT}/healthz" >/dev/null 2>&1; then
    echo "ok"
    echo "==> HeapBuddy is running at http://localhost:${PORT}"
    docker ps --filter "name=${NAME}" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
    exit 0
  fi
  echo -n "."
  sleep 1
done

echo
echo "!! Server did not become healthy in time; recent logs:" >&2
docker logs --tail 20 "${NAME}" >&2 || true
exit 1
