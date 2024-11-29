# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN apk add --no-cache gcc musl-dev

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o main .

# Final stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/main .

CMD ["./main"]
