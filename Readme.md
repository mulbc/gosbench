# Gosbench

Gosbench is the Golang reimplementation of [Cosbench](https://github.com/intel-cloud/cosbench).
It is a distributed S3 performance benchmark tool with [Prometheus exporter](https://opencensus.io/exporters/supported-exporters/go/prometheus/) leveraging the official [Golang AWS SDK](https://aws.amazon.com/sdk-for-go/)

## Usage

Gosbench consists of two parts:

* Server: Coordinates Workers and general test queue
* Workers: Actually connect to S3 and perform reading, writing, deleting and listing of objects

### Running a test

1. Build the server: `go install github.com/mulbc/gosbench/server`
1. Run the server, specifying a config file: `server -c path/to/config.yaml` - you can find an example config [in the example folder](examples/example_config.yaml)
1. The server will open port 2000 for workers to connect to - make sure this port is not blocked by your firewall!
1. Build the worker: `go install github.com/mulbc/gosbench/worker`
1. Run the worker, specifying the server connection details: `worker -s 192.168.1.1:2000`
1. The worker will immediately connect to the server and will start to get to work.
The worker opens port 8888 for the Prometheus exporter. Please make sure this port is allowed in your firewall and that you added the worker to the Prometheus config. An example prometheus.yaml config can be found [in the opencensus documentation](https://opencensus.io/exporters/supported-exporters/go/prometheus/)

### Evaluating a test

During a test, Prometheus will scrape the performance data continuously from the workers.
You can visualize this data in Grafana. To get an overview of what the provided data looks like, check out [the example scrape](examples/example_prom_exporter.log).

## Contributing

* Be aware that this repo uses pre-commit hooks - install them via `pre-commit install`
  * [More info](https://pre-commit.com/)
* We are using Go modules in this repository - read up on it [here](https://blog.golang.org/using-go-modules)
* Check out the open [TODOs](TODO.md) for hints on what to work on
