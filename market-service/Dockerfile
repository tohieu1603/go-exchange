FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY market-service/ market-service/
COPY go.work .
WORKDIR /app/market-service
RUN go build -o /bin/market-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/market-service /bin/market-service
ENTRYPOINT ["/bin/market-service"]
