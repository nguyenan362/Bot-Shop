FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot-shop ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bot-shop .
COPY --from=builder /app/internal/i18n/*.toml ./internal/i18n/
COPY --from=builder /app/web ./web
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080
CMD ["./bot-shop"]
