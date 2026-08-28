# syntax=docker/dockerfile:1.18@sha256:dabfc0969b935b2080555ace70ee69a5261af8a8f1b4df97b9e7fbcf6722eddf

FROM golang:1.26.7-alpine3.24@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/patchli-agent ./cmd/patchli-agent

# The patch agent intentionally runs with host-administration privileges when
# installed natively. This image is for isolated/container package management;
# do not mount the host root or Docker socket into it.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates
COPY --from=builder --chmod=0555 /out/patchli-agent /usr/local/bin/patchli-agent
ENTRYPOINT ["/usr/local/bin/patchli-agent"]
