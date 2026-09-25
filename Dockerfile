# Base images are pinned by digest, so the same git tag always builds from the same images.
FROM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /mqtt-load-generator ./cmd

FROM alpine:3@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

COPY --from=builder /mqtt-load-generator /mqtt-load-generator
WORKDIR /app

# Run as the "nobody" user that Alpine ships, not root. A numeric ID, because Kubernetes can
# only check runAsNonRoot against a number.
USER 65534:65534

ENTRYPOINT [ "/mqtt-load-generator" ]
