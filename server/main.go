package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"time"

	"github.com/mulbc/gosbench/common"
	"gopkg.in/yaml.v2"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())
}

var configFileLocation string
var readyWorkers chan net.Conn

func loadConfigFromFile() common.Testconf {
	flag.StringVar(&configFileLocation, "c", "", "Config file describing test run")
	flag.Parse()
	if configFileLocation == "" {
		log.Fatal("-c is a mandatory parameter - please specify the config file")
	}

	configFileContent, err := ioutil.ReadFile(configFileLocation)
	if err != nil {
		log.WithError(err).Fatalf("Error reading config file:")
	}

	var config common.Testconf
	err = yaml.Unmarshal(configFileContent, &config)
	if err != nil {
		log.WithError(err).Fatalf("Error unmarshaling config file:")
	}
	return config
}

func main() {
	// TODO:
	//  * Init gRPC
	//  * Create Grafana annotations when starting/stopping testcase
	//  *
	//
	config := loadConfigFromFile()
	common.CheckConfig(config)

	readyWorkers = make(chan net.Conn)
	defer close(readyWorkers)

	// common.InitS3(config)
	// for _, test := range config.Tests {
	// TODO send test to worker(s)
	// common.PerfTest(test)
	// }

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
		go func(c net.Conn) {
			log.Infof("%s connected to us ", c.RemoteAddr())
			decoder := json.NewDecoder(c)
			var message string
			err := decoder.Decode(&message)
			if err != nil {
				log.WithField("message", message).WithError(err).Error("Could not decode message, closing connection")
				c.Close()
				return
			}
			if message == "ready for work" {
				log.Debug("We have a new worker!")
				readyWorkers <- c
				return
			}
		}(conn)
		// Shut down the connection.
		// defer conn.Close()
	}
}

func scheduleTests(config common.Testconf) {

	for _, test := range config.Tests {
		workerConfig := &common.WorkerConf{
			Test: test,
		}
		preparedWorkers := make(chan bool, test.Workers)
		continueWorkers := make(chan bool, test.Workers)
		defer close(preparedWorkers)
		defer close(continueWorkers)

		for worker := 0; worker < test.Workers; worker++ {
			workerConfig.S3Config = config.S3Config[worker%len(config.S3Config)]
			workerConfig.WorkerID = fmt.Sprintf("w%d", worker)
			go executeTestOnWorker(<-readyWorkers, workerConfig, preparedWorkers, continueWorkers)
		}
		for worker := 0; worker < test.Workers; worker++ {
			<-preparedWorkers
		}
		for worker := 0; worker < test.Workers; worker++ {
			continueWorkers <- true
		}
	}
}

func executeTestOnWorker(conn net.Conn, config *common.WorkerConf, preparedWorkers chan bool, continueWorkers chan bool) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)
	encoder.Encode(config)

	var response string
	for {
		time.Sleep(time.Second)
		err := decoder.Decode(&response)
		if err == io.EOF {
			continue
		} else if err != nil {
			log.WithField("worker", config.WorkerID).WithField("message", response).WithError(err).Error("Worker responded unusually - dropping")
			conn.Close()
			return
		}
		switch response {
		case "preparations done":
			preparedWorkers <- true
			<-continueWorkers
			encoder.Encode("start work")
		case "work done":
			return
		}
	}
}
