FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./

RUN  go mod download
COPY operator/ ./operator/

RUN CGO_ENABLED=0 GOOS=linux go build -o crypto-operator ./operator/

FROM alpine:3.19
WORKDIR /root/

COPY --from=builder /app/crypto-operator ./crypto-operator
CMD ["./crypto-operator"]
