#!/usr/bin/env bash
go build -o cmd/load_generator/mqtt-load-generator cmd/load_generator/main.go
go build -o cmd/checker/checker cmd/checker/main.go