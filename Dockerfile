# syntax=docker/dockerfile:1
FROM golang:1.19.2-alpine3.16 AS builder

WORKDIR /app

COPY go.mod /app
COPY go.sum /app

RUN go mod download

COPY cmd/load_generator/main.go /app/load_generator/main.go
COPY cmd/checker/main.go /app/checker/main.go
COPY pkg/ /app/pkg

RUN CGO_ENABLED=0 go build -o /mqtt-load-generator load_generator/main.go && \
  go build -o /checker checker/main.go

FROM alpine:3

COPY --from=builder /mqtt-load-generator /mqtt-load-generator
COPY --from=builder /checker /checker
WORKDIR /app
ENTRYPOINT [ "/mqtt-load-generator" ]
