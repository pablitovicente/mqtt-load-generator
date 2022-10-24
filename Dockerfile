# syntax=docker/dockerfile:1
FROM golang:1.19.2-alpine3.16

WORKDIR /app

COPY go.mod /app
COPY go.sum /app

RUN go mod download

COPY cmd/load_generator/main.go /app
COPY pkg/ /app/pkg

RUN go build -o /mqtt-load-generator

ENTRYPOINT [ "/mqtt-load-generator" ]
