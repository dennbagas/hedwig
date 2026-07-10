# Stage 1: build
FROM golang:1.26.5 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o bot ./cmd/bot

# Stage 2: run
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
