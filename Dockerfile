FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./cmd/popsocket

FROM alpine:3.24

WORKDIR /app

COPY --from=builder /app/popsocket .

EXPOSE 8081

CMD ["./popsocket"]
