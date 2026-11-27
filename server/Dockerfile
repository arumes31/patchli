# Build context: repository root (the image also builds the agent binaries
# that the server hands to its installer).
# Compile natively on the build host for the target platform (no QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src/agent
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent/ ./
RUN for arch in amd64 arm64; do \
      CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" \
        -o /out/agents/rpatch-agent-linux-$arch ./cmd/rpatch-agent || exit 1; \
    done

WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -o /out/rpatch-server ./cmd/rpatch-server

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
WORKDIR /app
COPY --from=builder /out/rpatch-server .
COPY --from=builder /out/agents/ ./agents/

# Run as an unprivileged user.
USER 65532:65532
EXPOSE 8080
CMD ["./rpatch-server"]
