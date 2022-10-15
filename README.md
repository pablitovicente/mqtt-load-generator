# MQTT LOAD GENERATOR

A very simple MQTT load generator tool written in Go. The tool will first establish all the connections to the target broker
and only then start the publishing of messages.

## Requirements

- Go 1.19.1+ (probably works with older versions too!)

## Build

- `go build .` will generate the binary `mqtt-load-generator`

## Run

- `./mqtt-load-generator --help` will show all the supported options

Example to publish 1000 messages with 1KB payload size with a 1 ms wait between messages using 100 concurrent clients

```bash
./mqtt-load-generator  -c 1000 -s 1000 -t /golang/pub -i 1 -p 1883 -n 100 -u secret -P mega_secret -h localhost -p 1883
```

## TODO

- Use more idiomatic Go style
- Improve error handling
- Update project structure to use a cmd folder to have different commands for
  - publishing
  - counting received messages
- Add tests (this should be first :P)
