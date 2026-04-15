FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOBIN=/out go install -tags=postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S app \
  && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /src/db/migrations /app/db/migrations

RUN mkdir -p /app/storage && chown -R app:app /app

USER app

EXPOSE 8080

ENTRYPOINT ["/app/api"]
