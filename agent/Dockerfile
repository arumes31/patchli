FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN go build -ldflags "-X main.Version=${VERSION}" -o patchli-agent ./cmd/patchli-agent


# The agent manages host packages and therefore runs as root.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
WORKDIR /app
COPY --from=builder /app/patchli-agent .

CMD ["./patchli-agent"]
