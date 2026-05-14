FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o patchli-agent ./cmd/patchli-agent/main.go


FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/patchli-agent .

CMD ["./patchli-agent"]
