# Compile natively on the build host for the target platform (no QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags "-X main.Version=${VERSION}" -o rpatch-agent ./cmd/rpatch-agent


# The agent manages host packages and therefore runs as root.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
WORKDIR /app
COPY --from=builder /app/rpatch-agent .

CMD ["./rpatch-agent"]
