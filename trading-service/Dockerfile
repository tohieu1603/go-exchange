FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY trading-service/ trading-service/
COPY go.work .
WORKDIR /app/trading-service
RUN go build -o /bin/trading-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/trading-service /bin/trading-service
ENTRYPOINT ["/bin/trading-service"]
