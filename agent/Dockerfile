# Compile natively on the build host for the target platform (no QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags "-X main.Version=${VERSION}" -o patchli-agent ./cmd/patchli-agent


# The agent manages host packages and therefore runs as root.
FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6
WORKDIR /app
COPY --from=builder /app/patchli-agent .

CMD ["./patchli-agent"]
