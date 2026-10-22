FROM golang:1.27.2-alpine3.24@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN go build -ldflags "-X main.Version=${VERSION}" -o patchli-agent ./cmd/patchli-agent


# The agent manages host packages and therefore runs as root.
FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6
WORKDIR /app
COPY --from=builder /app/patchli-agent .

CMD ["./patchli-agent"]
