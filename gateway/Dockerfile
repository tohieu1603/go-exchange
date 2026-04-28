FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY gateway/ gateway/
COPY go.work .
WORKDIR /app/gateway
RUN go build -o /bin/gateway ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/gateway /bin/gateway
ENTRYPOINT ["/bin/gateway"]
