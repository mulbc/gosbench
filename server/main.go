package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"time"

	"github.com/mulbc/gosbench/common"
	"gopkg.in/yaml.v2"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())

	flag.StringVar(&configFileLocation, "c", "", "Config file describing test run")
	flag.IntVar(&serverPort, "p", 2000, "Port on which the server will be available for clients. Default: 2000")
	flag.BoolVar(&debug, "d", false, "enable debug log output")
	flag.BoolVar(&trace, "t", false, "enable trace log output")
	flag.Parse()
	// Only demand this flag if we are not running go test
	if configFileLocation == "" && flag.Lookup("test.v") == nil {
		log.Fatal("-c is a mandatory parameter - please specify the config file")
	}
	if debug {
		log.SetLevel(log.DebugLevel)
	} else if trace {
		log.SetLevel(log.TraceLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

var configFileLocation string
var serverPort int
var readyWorkers chan *net.Conn
var debug, trace bool

func loadConfigFromFile(configFileContent []byte) common.Testconf {
	var config common.Testconf
	err := yaml.Unmarshal(configFileContent, &config)
	if err != nil {
		log.WithError(err).Fatalf("Error unmarshaling config file:")
	}
	return config
}

func main() {
	configFileContent, err := ioutil.ReadFile(configFileLocation)
	if err != nil {
		log.WithError(err).Fatalf("Error reading config file:")
	}
	config := loadConfigFromFile(configFileContent)
	common.CheckConfig(config)

	readyWorkers = make(chan *net.Conn)
	defer close(readyWorkers)

	// Listen on TCP port 2000 on all available unicast and
	// anycast IP addresses of the local system.
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.WithError(err).Fatal("Could not open port!")
	}
	defer l.Close()
	log.Info("Ready to accept connections")
	go scheduleTests(config)
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.WithError(err).Fatal("Issue when waiting for connection of clients")
		}
		// Handle the connection in a new goroutine.
		// The loop then returns to accepting, so that
		// multiple connections may be served concurrently.
		go func(c *net.Conn) {
			log.Infof("%s connected to us ", (*c).RemoteAddr())
			decoder := json.NewDecoder(*c)
			var message string
			err := decoder.Decode(&message)
			if err != nil {
				log.WithField("message", message).WithError(err).Error("Could not decode message, closing connection")
				(*c).Close()
				return
			}
			if message == "ready for work" {
				log.Debug("We have a new worker!")
				readyWorkers <- c
				return
			}
		}(&conn)
		// Shut down the connection.
		// defer conn.Close()
	}
}

func scheduleTests(config common.Testconf) {

	for testNumber, test := range config.Tests {

		doneChannel := make(chan bool, test.Workers)
		resultChannel := make(chan common.BenchmarkResult, test.Workers)
		continueWorkers := make(chan bool, test.Workers)
		defer close(doneChannel)
		defer close(continueWorkers)

		for worker := 0; worker < test.Workers; worker++ {
			workerConfig := &common.WorkerConf{
				Test:     test,
				S3Config: config.S3Config[worker%len(config.S3Config)],
				WorkerID: fmt.Sprintf("w%d", worker),
			}
			workerConnection := <-readyWorkers
			log.WithField("Worker", (*workerConnection).RemoteAddr()).Infof("We found worker %d / %d for test %d", worker+1, test.Workers, testNumber)
			go executeTestOnWorker(workerConnection, workerConfig, doneChannel, continueWorkers, resultChannel)
		}
		for worker := 0; worker < test.Workers; worker++ {
			// Will halt until all workers are done with preparations
			<-doneChannel
		}
		// Add sleep after prep phase so that drives can relax
		time.Sleep(5 * time.Second)
		log.WithField("test", test.Name).Info("All workers have finished preparations - starting performance test")
		startTime := time.Now().UTC()
		for worker := 0; worker < test.Workers; worker++ {
			continueWorkers <- true
		}
		var benchResults []common.BenchmarkResult
		for worker := 0; worker < test.Workers; worker++ {
			// Will halt until all workers are done with their work
			<-doneChannel
			benchResults = append(benchResults, <-resultChannel)
		}
		log.WithField("test", test.Name).Info("All workers have finished the performance test - continuing with next test")
		stopTime := time.Now().UTC()
		log.WithField("test", test.Name).Infof("GRAFANA: ?from=%d&to=%d", startTime.UnixNano()/int64(1000000), stopTime.UnixNano()/int64(1000000))
		benchResult := sumBenchmarkResults(benchResults)
		benchResult.Duration = stopTime.Sub(startTime)
		log.WithField("test", test.Name).
			WithField("Total Operations", benchResult.Operations).
			WithField("Total Bytes", benchResult.Bytes).
			WithField("Average BW in Byte/s", benchResult.Bandwidth).
			WithField("Average latency in ms", benchResult.LatencyAvg).
			WithField("Test runtime on server", benchResult.Duration).
			Infof("PERF RESULTS")
	}
	log.Info("All performance tests finished")
	for {
		workerConnection := <-readyWorkers
		shutdownWorker(workerConnection)
	}
}

func executeTestOnWorker(conn *net.Conn, config *common.WorkerConf, doneChannel chan bool, continueWorkers chan bool, resultChannel chan common.BenchmarkResult) {
	encoder := json.NewEncoder(*conn)
	decoder := json.NewDecoder(*conn)
	_ = encoder.Encode(common.WorkerMessage{Message: "init", Config: config})

	var response common.WorkerMessage
	for {
		err := decoder.Decode(&response)
		if err != nil {
			log.WithField("worker", config.WorkerID).WithField("message", response).WithError(err).Error("Worker responded unusually - dropping")
			(*conn).Close()
			return
		}
		log.Tracef("Response: %+v", response)
		switch response.Message {
		case "preparations done":
			doneChannel <- true
			<-continueWorkers
			_ = encoder.Encode(common.WorkerMessage{Message: "start work"})
		case "work done":
			doneChannel <- true
			resultChannel <- response.BenchResult
			(*conn).Close()
			return
		}
	}
}

func shutdownWorker(conn *net.Conn) {
	encoder := json.NewEncoder(*conn)
	log.WithField("Worker", (*conn).RemoteAddr()).Info("Shutting down worker")
	_ = encoder.Encode(common.WorkerMessage{Message: "shutdown"})
}

func sumBenchmarkResults(results []common.BenchmarkResult) common.BenchmarkResult {
	sum := common.BenchmarkResult{}
	bandwidthAverages := float64(0)
	latencyAverages := float64(0)
	for _, result := range results {
		sum.Bytes += result.Bytes
		sum.Operations += result.Operations
		latencyAverages += result.LatencyAvg
		bandwidthAverages += result.Bandwidth
	}
	sum.LatencyAvg = latencyAverages / float64(len(results))
	sum.TestName = results[0].TestName
	sum.Bandwidth = bandwidthAverages
	return sum
}
