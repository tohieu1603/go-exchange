FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY es-indexer/ es-indexer/
COPY go.work .
WORKDIR /app/es-indexer
RUN go build -o /bin/es-indexer ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/es-indexer /bin/es-indexer
ENTRYPOINT ["/bin/es-indexer"]
