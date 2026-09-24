# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /mqtt-load-generator ./cmd

FROM alpine:3

COPY --from=builder /mqtt-load-generator /mqtt-load-generator
WORKDIR /app
ENTRYPOINT [ "/mqtt-load-generator" ]
