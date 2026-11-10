# Compile natively on the build host for the target platform (no QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o patchli-server ./cmd/patchli-server/main.go

FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6
WORKDIR /app
COPY --from=builder /app/patchli-server .

# Run as an unprivileged user.
USER 65532:65532
EXPOSE 8080
CMD ["./patchli-server"]
