# syntax=docker/dockerfile:1
#
# Multi-stage build producing a single static binary that serves the embedded
# React SPA and the JSON API on one port.

# 1) Build the frontend SPA.
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
# Prefer a reproducible install from the lockfile when present and consistent;
# fall back to a fresh resolve otherwise (no lockfile committed yet).
RUN if [ -f package-lock.json ]; then npm ci || npm install; else npm install; fi
COPY frontend/ ./
RUN npm run build

# 2) Build the Go binary with the SPA embedded (mirrors `make build`).
FROM golang:1.22-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/frontend/dist/client/assets ./internal/server/webui/assets
COPY --from=frontend /app/frontend/dist/client/_shell.html ./internal/server/webui/_shell.html
ARG VERSION=docker
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
      -X github.com/sachin-handiekar/HeapBuddy/cmd.version=${VERSION} \
      -X github.com/sachin-handiekar/HeapBuddy/cmd.commit=${COMMIT} \
      -X github.com/sachin-handiekar/HeapBuddy/cmd.date=${DATE}" \
    -o /heapbuddy .

# 3) Minimal, non-root runtime.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /heapbuddy /heapbuddy
# A container must bind all interfaces; the server prints an exposure warning at
# startup since it is unauthenticated. Run it inside a trusted network only.
ENV HEAPBUDDY_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/heapbuddy"]
CMD ["serve"]
