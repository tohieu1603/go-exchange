FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY notification-service/ notification-service/
COPY go.work .
WORKDIR /app/notification-service
RUN go build -o /bin/notification-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/notification-service /bin/notification-service
ENTRYPOINT ["/bin/notification-service"]
