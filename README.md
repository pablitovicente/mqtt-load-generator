# MQTT Load Generator

A very simple MQTT load generator tool written in Go. The tool will first establish all the connections to the target broker
and only then start the publishing of messages.

A simple checker that counts received messages is also provided.

## Requirements

- Go 1.19.1+ (probably works with older versions too!)

## Build

- `./build_all.sh` will generate the binaries
  - `cmd/load_generator/mqtt-load-generator`
  - `cmd/checker/checker`

## Run

- `./cmd/load_generator/mqtt-load-generator --help` will show all the supported options for the load generator
- `./cmd/checker/checker --help` will show all the supported options for the checker

Example to publish 1000 messages with 1KB payload size with a 1 ms wait between messages using 100 concurrent clients

```bash
./cmd/load_generator/mqtt-load-generator  -c 1000 -s 1000 -t /golang/pub -i 1 -n 100 -u secret -P mega_secret -h localhost -p 1883
```

To count incoming messages you can use the checker

```bash
./cmd/checker/checker -h localhost -p 1883 -u secret -P ultra_secret -t /golang/pub
```

## Docker image

To create a docker image for the load generator:

```bash
docker build -t mqtt-load-generator .
```

To run a docker container from it reuse the same command line like above with:

```bash
docker run --rm -it mqtt-load-generator -c 1000 -s 1000 -t /golang/pub -i 1 -n 100 -u secret -P mega_secret -h localhost -p 1883 
```

To run the image in a Kubernetes cluster 
(e.g., to creating the load nearer to cluster resources by using cluster-local addresses):

```bash
kubectl run mqtt-load-generator --image=jforge/mqtt-load-generator  \
  -- -h <mqtt-broker-address> -p 1883 -u secret -P mega_secret \
     -c 1000 -s 1000 -t /golang/pub -i 1 -n 100
```

## TODO

- Use more idiomatic Go style
- Support TLS
- Support mTlS
- Improve error handling
- Add tests (this should be first :P)
