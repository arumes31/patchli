FROM golang:1.23-alpine AS builder

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
