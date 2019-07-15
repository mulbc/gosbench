package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/mulbc/gosbench/common"
	log "github.com/sirupsen/logrus"
)

var config common.WorkerConf

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var serverAddress string
	flag.StringVar(&serverAddress, "s", "", "Gosbench Server IP and Port in the form '192.168.1.1:2000'")
	flag.Parse()
	if serverAddress == "" {
		log.Fatal("-s is a mandatory parameter - please specify the server IP and Port")
	}

	for {
		err := connectToServer(serverAddress)
		if err != nil {
			log.WithError(err).Fatal("Issues with server connection")
		}
	}
}

func connectToServer(serverAddress string) error {
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		log.WithError(err).Fatal("Could not connect to the server")
	}
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	encoder.Encode("ready for work")

	var response common.WorkerMessage
	Workqueue := &Workqueue{
		Queue: &[]WorkItem{},
	}
	for {
		err := decoder.Decode(&response)
		if err != nil {
			log.WithField("message", response).WithError(err).Error("Server responded unusually - reconnecting")
			conn.Close()
			return nil
		}
		log.Tracef("Response: %+v", response)
		switch response.Message {
		case "init":
			config = *response.Config
			log.Info("Got config from server - starting preparations now")

			InitS3(*config.S3Config)
			fillWorkqueue(config.Test, Workqueue, config.WorkerID)

			for _, work := range *Workqueue.Queue {
				work.Prepare()
			}
			log.Info("Preparations finished - waiting on server to start work")
			encoder.Encode(common.WorkerMessage{Message: "preparations done"})
		case "start work":
			if config == (common.WorkerConf{}) || len(*Workqueue.Queue) == 0 {
				log.Fatal("Was instructed to start work - but the preparation step is incomplete - reconnecting")
				return nil
			}
			log.Info("Starting to work")
			PerfTest(config.Test, Workqueue)
			encoder.Encode(common.WorkerMessage{Message: "work done"})
			// Work is done - return to being a ready worker by reconnecting
			return nil
		}
	}
}

// PerfTest runs a performance test as configured in testConfig
func PerfTest(testConfig *common.TestCaseConfiguration, Workqueue *Workqueue) {
	workChannel := make(chan WorkItem, len(*Workqueue.Queue))
	doneChannel := make(chan bool)
	for worker := 0; worker < testConfig.ParallelClients; worker++ {
		go DoWork(workChannel, doneChannel)
	}
	log.Infof("Started %d parallel clients", testConfig.ParallelClients)
	if testConfig.Runtime != 0 {
		workUntilTimeout(Workqueue, workChannel, testConfig.Runtime)
	} else {
		workUntilOps(Workqueue, workChannel, testConfig.OpsDeadline, testConfig.ParallelClients)
	}
	// Wait for all the goroutines to finish
	for i := 0; i < testConfig.ParallelClients; i++ {
		<-doneChannel
	}
	log.Info("All clients finished")
	if testConfig.CleanAfter {
		log.Info("Housekeeping started")
		for _, work := range *Workqueue.Queue {
			work.Clean()
		}
		log.Info("Housekeeping finished")
	}
}

func workUntilTimeout(Workqueue *Workqueue, workChannel chan WorkItem, runtime time.Duration) {
	timer := time.NewTimer(runtime)
	for {
		for _, work := range *Workqueue.Queue {
			select {
			case <-timer.C:
				log.Debug("Reached Runtime end")
				WorkCancel()
				return
			case workChannel <- work:
			}
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.Prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func workUntilOps(Workqueue *Workqueue, workChannel chan WorkItem, maxOps uint64, numberOfWorker int) {
	currentOps := uint64(0)
	for {
		for _, work := range *Workqueue.Queue {
			if currentOps >= maxOps {
				log.Debug("Reached OpsDeadline ... waiting for workers to finish")
				for worker := 0; worker < numberOfWorker; worker++ {
					workChannel <- Stopper{}
				}
				return
			}
			currentOps++
			workChannel <- work
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.Prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func fillWorkqueue(testConfig *common.TestCaseConfiguration, Workqueue *Workqueue, workerID string) {

	if testConfig.ReadWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "read"})
	}
	if testConfig.WriteWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "write"})
	}
	if testConfig.ListWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "list"})
	}
	if testConfig.DeleteWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "delete"})
	}

	bucketCount := common.EvaluateDistribution(testConfig.Buckets.NumberMin, testConfig.Buckets.NumberMax, &testConfig.Buckets.NumberLast, 1, testConfig.Buckets.NumberDistribution)
	for bucket := uint64(0); bucket < bucketCount; bucket++ {
		objectCount := common.EvaluateDistribution(testConfig.Objects.NumberMin, testConfig.Objects.NumberMax, &testConfig.Objects.NumberLast, 1, testConfig.Objects.NumberDistribution)
		for object := uint64(0); object < objectCount; object++ {
			objectSize := common.EvaluateDistribution(testConfig.Objects.SizeMin, testConfig.Objects.SizeMax, &testConfig.Objects.SizeLast, 1, testConfig.Objects.SizeDistribution)

			nextOp := GetNextOperation(Workqueue)
			switch nextOp {
			case "read":
				IncreaseOperationValue(nextOp, 1/float64(testConfig.ReadWeight), Workqueue)
				new := ReadOperation{
					Bucket:     fmt.Sprintf("%s%s%d", workerID, testConfig.BucketPrefix, bucket),
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "write":
				IncreaseOperationValue(nextOp, 1/float64(testConfig.WriteWeight), Workqueue)
				new := WriteOperation{
					Bucket:     fmt.Sprintf("%s%s%d", workerID, testConfig.BucketPrefix, bucket),
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "list":
				IncreaseOperationValue(nextOp, 1/float64(testConfig.ListWeight), Workqueue)
				new := ListOperation{
					Bucket:     fmt.Sprintf("%s%s%d", workerID, testConfig.BucketPrefix, bucket),
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "delete":
				IncreaseOperationValue(nextOp, 1/float64(testConfig.DeleteWeight), Workqueue)
				new := DeleteOperation{
					Bucket:     fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					ObjectName: fmt.Sprintf("%s%d", testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			}
		}
	}
}
