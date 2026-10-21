FROM golang:1.26.10-alpine3.24@sha256:9ce231b28998b32e116927c61aa5c406183061d4f3f2d61b292c88e7f6bdf7ca AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o patchli-server ./cmd/patchli-server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/patchli-server .

EXPOSE 8080
CMD ["./patchli-server"]
