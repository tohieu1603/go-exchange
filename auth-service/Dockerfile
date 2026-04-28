FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY auth-service/ auth-service/
COPY go.work .
WORKDIR /app/auth-service
RUN go build -o /bin/auth-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/auth-service /bin/auth-service
ENTRYPOINT ["/bin/auth-service"]
