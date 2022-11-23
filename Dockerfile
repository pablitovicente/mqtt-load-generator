# syntax=docker/dockerfile:1
FROM golang:1.19.2-alpine3.16 AS builder

WORKDIR /app

COPY go.mod /app
COPY go.sum /app

RUN go mod download

COPY cmd/load_generator/main.go /app
COPY pkg/ /app/pkg

RUN CGO_ENABLED=0 go build -o /mqtt-load-generator

FROM alpine:3

COPY --from=builder /mqtt-load-generator /mqtt-load-generator

WORKDIR /app
ENTRYPOINT [ "/mqtt-load-generator" ]
