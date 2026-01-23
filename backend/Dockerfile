FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /backend .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY --from=builder /backend .
COPY --from=builder /app/services/templates ./services/templates

EXPOSE 8080

CMD ["./backend"]
