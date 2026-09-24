# MQTT Load Generator

An MQTT load generator written in Go. One binary, `mqtt-load-generator`, with three
subcommands:

- `pub` publishes messages. It connects all clients first, then starts publishing. Running
  the binary with no subcommand also runs `pub`.
- `sub` subscribes to a topic and counts received messages, shown as a progress bar or as log
  lines (`--disable-bar`).
- `dump` subscribes to a topic and prints each received payload as text, one per line, on
  stdout.

## Requirements

- Go 1.27 or newer

## Build

```bash
make build
```

This builds `bin/mqtt-load-generator` for the current OS and architecture. To build for
another platform, set `GOOS`/`GOARCH`. For a Raspberry Pi with 64-bit Raspberry Pi OS:

```bash
make build GOARCH=arm64
```

For 32-bit Raspberry Pi OS:

```bash
make build GOARCH=arm GOARM=7
```

## Development

| Target | Does |
|---|---|
| `make test` | Runs the tests with the race detector |
| `make lint` | Runs golangci-lint if it is installed, otherwise `go vet` |
| `make nilaway` | Checks for possible nil pointer panics with nilaway |
| `make check` | Runs `lint`, `nilaway` and `test` |
| `make docker` | Builds the Docker image |

`make nilaway` downloads and builds nilaway on first use, so it needs no install.

`make lint` needs golangci-lint v2 built with Go 1.27 or newer. A copy built with an older
Go refuses to check this module. Install it with:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

The golangci-lint site (golangci-lint.run) lists other ways to install it.

## Run

`-h` is short for `--host`, as in `mosquitto_pub`. Help is `--help` only.

```bash
./bin/mqtt-load-generator --help
./bin/mqtt-load-generator pub --help
./bin/mqtt-load-generator sub --help
./bin/mqtt-load-generator dump --help
```

### Publish (`pub`, or no subcommand)

Publish 1000 messages of 1000 bytes from each of 100 clients, waiting 1 ms between messages:

```bash
./bin/mqtt-load-generator -c 1000 -s 1000 -t /golang/pub -i 1 -n 100 -h localhost -p 1883 \
  -u secret -P mega_secret
```

The same run with the subcommand and long flag names, and the password from an environment
variable (see "Credentials and TLS"):

```bash
export MQTT_PASSWORD=mega_secret
./bin/mqtt-load-generator pub --count 1000 --size 1000 --topic /golang/pub --interval 1 \
  --clients 100 --host localhost --port 1883 --username secret
```

The exit code is non-zero when any publish failed or timed out.

### Subscribe (`sub`)

```bash
./bin/mqtt-load-generator sub -h localhost -p 1883 -u secret -P mega_secret -t /golang/pub
```

With `--disable-bar`, counts are printed as log lines instead of a progress bar. Use it when
the output is not a terminal, for example in a container or in CI.

### Dump (`dump`)

Print every payload received on a topic, one per line:

```bash
./bin/mqtt-load-generator dump -h localhost -p 1883 -t /golang/pub
```

With `--show-topic`, each line is `topic<TAB>payload`. The separator is a tab because MQTT
topics can contain spaces.

## Credentials and TLS

Username, password and the three TLS file paths can be given as flags or as environment
variables. A flag given on the command line is used instead of the environment variable.

| Flag | Environment variable |
|---|---|
| `-u`, `--username` | `MQTT_USERNAME` |
| `-P`, `--password` | `MQTT_PASSWORD` |
| `--ca` | `MQTT_CA` |
| `--cert` | `MQTT_CERT` |
| `--key` | `MQTT_KEY` |

A password given with `-P` is visible to every user on the machine in `ps` output.
`MQTT_PASSWORD` is not. In Kubernetes, it can come from a Secret (see below).

As a flag:

```bash
./bin/mqtt-load-generator sub -h broker -u secret -P mega_secret -t /golang/pub
```

As environment variables:

```bash
export MQTT_USERNAME=secret
export MQTT_PASSWORD=mega_secret
./bin/mqtt-load-generator sub -h broker -t /golang/pub
```

`--cert`, `--ca` and `--key` must be given together or not at all. Giving only one or two of
them is an error.

## Throughput: `--inflight`, `--ack-timeout`, `--connect-concurrency`

`--inflight` is how many publishes one client can have sent but not yet acknowledged. With
the default, 1, each client waits for the acknowledgement of a message before sending the
next one. This is how 1.x worked. Higher values let a client keep sending while earlier
messages are still waiting for their acknowledgement. The maximum is 65535, because MQTT
message IDs are 16 bits. At QoS 0 there are no acknowledgements, so `--inflight` has no
effect there.

`--ack-timeout` (default 30s) is how long to wait for an acknowledgement before counting the
publish as timed out. With a high `--inflight` on a busy broker, acknowledgements take longer;
raise this if runs report timeouts.

`--connect-concurrency` (default 16) is how many clients connect at the same time before
publishing starts. Some brokers limit how fast new connections can arrive, or run a slow
authentication check for each one. `--connect-concurrency 1` connects one client at a time,
as 1.x did.

## `--ordered` on `sub` and `dump`

With `--ordered`, received messages are handled one at a time, in the order they arrive.
Without it, they are handled in parallel and the order can change.

Ordered is slower. Each message waits until the one before it has been handled, so under
heavy load the tool can fall behind the broker.

| Command | Default | Reason |
|---|---|---|
| `sub` | off | Counting does not depend on order |
| `dump` | on | Printed lines are read by a person |

Turn it on for `sub` with `--ordered`, or off for `dump` with `--ordered=false`.

## `--benchmark` payloads

With `--benchmark`, each payload is JSON instead of random bytes:

```json
{"timestamp":1699999999999,"padding":"xxxx..."}
```

`timestamp` is the publish time in Unix milliseconds. `padding` fills the payload up to
`--size` bytes, and is empty when `--size` is below 50. The format does not change between
versions, because some collectors read `timestamp` from it to measure latency. `sub` does not
compute latency.

## Docker

Build:

```bash
docker build -t mqtt-load-generator .
```

Run `pub` with 1.x-style arguments:

```bash
docker run --rm -e MQTT_PASSWORD=mega_secret mqtt-load-generator \
  -c 1000 -s 1000 -t /golang/pub -i 1 -n 100 -u secret -h mqtt-broker -p 1883
```

Run `sub`:

```bash
docker run --rm -e MQTT_PASSWORD=mega_secret mqtt-load-generator \
  sub -h mqtt-broker -p 1883 -u secret -t /golang/pub --disable-bar
```

## Kubernetes

`k8s/job.yaml` runs `pub` as a Job. `k8s/checker-deployment.yaml` runs `sub` as a
Deployment. Both read the MQTT password from the Secret in `k8s/secret.yaml`. Change the
placeholder password in that file before applying it.

```bash
kubectl create -f k8s/secret.yaml
kubectl create -f k8s/job.yaml
kubectl create -f k8s/checker-deployment.yaml
```

Follow the Job's output:

```bash
kubectl logs -f -l job-name=mqtt-load-generator
```

Run the Job again after it finished:

```bash
kubectl delete jobs.batch -l app=mqtt-load-generator ; kubectl create -f k8s/job.yaml
```

To run several Jobs in parallel, set `spec.parallelism` in `k8s/job.yaml`. To remove all Jobs
from this tool:

```bash
kubectl delete jobs.batch -l app=mqtt-load-generator
```

## Upgrading from 1.x

2.0.0 changes some flags and commands. The "2.0.0" section of `CHANGELOG.md` lists every
change. 1.x releases remain available.

## TODO

- Safe output for `dump`: escape control characters in payloads and topics
- Detailed statistics per client and in total

## Contributors

<a href="https://github.com/pablitovicente/mqtt-load-generator/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=pablitovicente/mqtt-load-generator" />
</a>
