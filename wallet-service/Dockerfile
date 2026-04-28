FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY wallet-service/ wallet-service/
COPY go.work .
WORKDIR /app/wallet-service
RUN go build -o /bin/wallet-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/wallet-service /bin/wallet-service
ENTRYPOINT ["/bin/wallet-service"]
