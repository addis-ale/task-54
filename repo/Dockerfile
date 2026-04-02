FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /bin/server ./cmd/server
RUN CGO_ENABLED=1 go build -o /bin/seed   ./cmd/seed

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/server /app/server
COPY --from=builder /bin/seed   /app/seed

WORKDIR /app

RUN mkdir -p /data/media /data/logs /data/diagnostics /data/shared_reports

EXPOSE 8080

ENTRYPOINT ["/app/server"]
