FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./cmd/popsocket

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/popsocket .

EXPOSE 8081

CMD ["./popsocket"]
