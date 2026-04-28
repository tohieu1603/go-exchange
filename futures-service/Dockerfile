FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ shared/
COPY futures-service/ futures-service/
COPY go.work .
WORKDIR /app/futures-service
RUN go build -o /bin/futures-service ./cmd/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/futures-service /bin/futures-service
ENTRYPOINT ["/bin/futures-service"]
