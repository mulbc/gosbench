package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/mulbc/gosbench/common"
	"gopkg.in/yaml.v2"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())

	flag.StringVar(&configFileLocation, "c", "", "Config file describing test run")
	flag.Parse()
	// Only demand this flag if we are not running go test
	if configFileLocation == "" && flag.Lookup("test.v") == nil {
		log.Fatal("-c is a mandatory parameter - please specify the config file")
	}
}

var configFileLocation string
var readyWorkers chan *net.Conn

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
	l, err := net.Listen("tcp", ":2000")
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
			log.WithField("Worker", (*workerConnection).RemoteAddr()).Infof("We found worker %d / %d for test %d", worker, test.Workers, testNumber)
			go executeTestOnWorker(workerConnection, workerConfig, doneChannel, continueWorkers)
		}
		for worker := 0; worker < test.Workers; worker++ {
			// Will halt until all workers are done with preparations
			<-doneChannel
		}
		log.WithField("test", testNumber).Info("All workers have finished preparations - starting performance test")
		startTime := time.Now().UTC().UnixNano() / int64(1000000)
		// Add sleep after prep phase so that drives can relax
		time.Sleep(5 * time.Second)
		for worker := 0; worker < test.Workers; worker++ {
			continueWorkers <- true
		}
		for worker := 0; worker < test.Workers; worker++ {
			// Will halt until all workers are done with their work
			<-doneChannel
		}
		log.WithField("test", testNumber).Info("All workers have finished the performance test - continuing with next test")
		stopTime := time.Now().UTC().UnixNano() / int64(1000000)
		log.WithField("test", testNumber).Infof("GRAFANA: ?from=%d&to=%d", startTime, stopTime)
	}
	log.Info("All performance tests finished")
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func executeTestOnWorker(conn *net.Conn, config *common.WorkerConf, doneChannel chan bool, continueWorkers chan bool) {
	encoder := json.NewEncoder(*conn)
	decoder := json.NewDecoder(*conn)
	encoder.Encode(common.WorkerMessage{Message: "init", Config: config})

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
			encoder.Encode(common.WorkerMessage{Message: "start work"})
		case "work done":
			doneChannel <- true
			(*conn).Close()
			return
		}
	}
}
