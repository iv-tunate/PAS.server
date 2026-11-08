# syntax=docker/dockerfile:1

# ---- build ------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server . \
  && CGO_ENABLED=0 GOOS=linux go install -ldflags="-s -w" github.com/pressly/goose/v3/cmd/goose@v3.20.0 \
  && cp /go/bin/goose /out/goose

# ---- run --------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
  && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/goose /usr/local/bin/goose
COPY internal/db/migrations /app/migrations

USER app
ENV PORT=8080
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/health" >/dev/null || exit 1

CMD ["/app/server"]