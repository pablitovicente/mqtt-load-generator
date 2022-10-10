# mqtt-benchmark

A very simple MQTT benchmark tool written in Go

It's very light on resources as it is intended to be run multiple times to benchmark MQTT brokers and to easily generate multiple loads by simply running the command multiple times.
Future iterations will switch to use Go routines instead of requiring multiple runs.

## Requirements

- Go 1.19.1+ (probably works with older versions too!)

## Build

- `go build .` will generate the binary `mqtt-benchmark`

## Run

- `./mqtt-benchmark --help` will show all the supported options

Example to publish 100,000 messages with 1KB payload size with a 1 ms wait between messages using an MQTT client id of go_mqtt_1

```bash
/mqtt-benchmark -c 100000 -s 1000 -t /golang/pub -u secret -P mega_secret -i 1 -id go_mqtt_1 -h localhost -p 1883
```
