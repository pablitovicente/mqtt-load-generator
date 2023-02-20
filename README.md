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

To check all the available options run

```bash
./cmd/load_generator/mqtt-load-generator --help
```

## Docker image

To create a docker image for the load generator:

```bash
docker build -t mqtt-load-generator .
```

### Running the load generator

To run a docker container from it reuse the same command line like above with:

```bash
docker run --rm -it mqtt-load-generator -c 1000 -s 1000 -t /golang/pub -i 1 -n 100 -u secret -P mega_secret -h localhost -p 1883 
```

### Running the checker

```bash
docker run --entrypoint /checker --rm -it mqtt-load-generator -t /golang/pub -u secret -P mega_secret -h localhost -p 1883 
```

## Run on Kubernetes
Here are a few methods of running the image in a Kubernetes cluster
(e.g., to creating the load nearer to cluster resources by using cluster-local addresses):

### Running the load generator

```bash
kubectl run mqtt-load-generator --image=jforge/mqtt-load-generator  \
  -- -h <mqtt-broker-address> -p 1883 -u secret -P mega_secret \
     -c 1000 -s 1000 -t /golang/pub -i 1 -n 100
```

### Running the load checker

```bash
kubectl run mqtt-load-checker --image=pgschk/mqtt-load-generator --command \
  -- /checker -h <mqtt-broker-address> -p 1883 -u secret -P mega_secret \
      -t /golang/pub --disable-bar
```

### Running the load generator as a Kubernetes Job
To run as a Kubernetes job you can adjust the file `k8s/job.yaml` and apply it with:
 
 ```bash
 kubectl create -f k8s/job.yaml
 ```

To view the log output use: 
```bash
kubectl logs -f -l job-name=mqtt-load-generator
```

To restart when one run is finished you can use:
```bash
kubectl delete jobs.batch -l app=mqtt-load-generator ; kubectl create -f k8s/job.yaml
```

**Run multiple jobs parallel**

To run multiple jobs in parallel you adjust parameter `spec.parallelism` in `k8s/job.yaml`

**Delete multiple jobs**

To clean up all mqtt-load-generator jobs you can run:
```bash
kubectl delete jobs.batch -l app=mqtt-load-generator
```

### Running the load checker as Kubernetes Deployment
 ```bash
 kubectl create -f k8s/checker-deployment.yaml
 ```

## TODO

- Use more idiomatic Go style
- Support TLS
- Support mTlS
- Improve error handling
- Add tests (this should be first :P)

## Contributors

<a href="https://github.com/pablitovicente/mqtt-load-generator/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=pablitovicente/mqtt-load-generator" />
</a>