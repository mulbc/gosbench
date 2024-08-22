package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mulbc/gosbench/common"
	log "github.com/sirupsen/logrus"
)

var config common.WorkerConf
var prometheusPort int
var debug, trace bool

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var serverAddress string
	flag.StringVar(&serverAddress, "s", "", "Gosbench Server IP and Port in the form '192.168.1.1:2000'")
	flag.IntVar(&prometheusPort, "p", 8888, "Port on which the Prometheus Exporter will be available. Default: 8888")
	flag.BoolVar(&debug, "d", false, "enable debug log output")
	flag.BoolVar(&trace, "t", false, "enable trace log output")
	flag.Parse()
	if serverAddress == "" {
		log.Fatal("-s is a mandatory parameter - please specify the server IP and Port")
	}

	if debug {
		log.SetLevel(log.DebugLevel)
	} else if trace {
		log.SetLevel(log.TraceLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	for {
		err := connectToServer(serverAddress)
		if err != nil {
			log.WithError(err).Error("Issues with server connection")
			time.Sleep(time.Second)
		}
	}
}

func connectToServer(serverAddress string) error {
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		// return errors.New("Could not establish connection to server yet")
		return err
	}
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	_ = encoder.Encode("ready for work")

	var response common.WorkerMessage
	Workqueue := &Workqueue{
		Queue: &[]WorkItem{},
	}
	for {
		err := decoder.Decode(&response)
		if err != nil {
			log.WithField("message", response).WithError(err).Error("Server responded unusually - reconnecting")
			conn.Close()
			return errors.New("Issue when receiving work from server")
		}
		log.Tracef("Response: %+v", response)
		switch response.Message {
		case "init":
			config = *response.Config
			log.Info("Got config from server - starting preparations now")

			InitS3(*config.S3Config)
			fillWorkqueue(config.Test, Workqueue, config.WorkerID, config.Test.WorkerShareBuckets)

			for _, work := range *Workqueue.Queue {
				err = work.Prepare()
				if err != nil {
					log.WithError(err).Error("Error during work preparation - ignoring")
				}
			}
			log.Info("Preparations finished - waiting on server to start work")
			_ = encoder.Encode(common.WorkerMessage{Message: "preparations done"})
		case "start work":
			if config == (common.WorkerConf{}) || len(*Workqueue.Queue) == 0 {
				log.Fatal("Was instructed to start work - but the preparation step is incomplete - reconnecting")
				return nil
			}
			log.Info("Starting to work")
			duration := PerfTest(config.Test, Workqueue, config.WorkerID)
			benchResults := getCurrentPromValues(config.Test.Name)
			benchResults.Duration = duration
			benchResults.Bandwidth = benchResults.Bytes / duration.Seconds()
			log.Infof("PROM VALUES %+v", benchResults)
			_ = encoder.Encode(common.WorkerMessage{Message: "work done", BenchResult: benchResults})
			// Work is done - return to being a ready worker by reconnecting
			return nil
		case "shutdown":
			log.Info("Server told us to shut down - all work is done for today")
			os.Exit(0)
		}
	}
}

// PerfTest runs a performance test as configured in testConfig
func PerfTest(testConfig *common.TestCaseConfiguration, Workqueue *Workqueue, workerID string) time.Duration {
	workChannel := make(chan WorkItem, len(*Workqueue.Queue))
	notifyChan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(testConfig.ParallelClients)

	startTime := time.Now().UTC()
	promTestStart.WithLabelValues(testConfig.Name).Set(float64(startTime.UnixNano() / int64(1000000)))
	// promTestGauge.WithLabelValues(testConfig.Name).Inc()
	for worker := 0; worker < testConfig.ParallelClients; worker++ {
		go DoWork(workChannel, notifyChan, wg)
	}
	log.Infof("Started %d parallel clients", testConfig.ParallelClients)
	if testConfig.Runtime != 0 {
		workUntilTimeout(Workqueue, workChannel, notifyChan, time.Duration(testConfig.Runtime))
	} else {
		workUntilOps(Workqueue, workChannel, testConfig.OpsDeadline, testConfig.ParallelClients)
	}
	// Wait for all the goroutines to finish
	wg.Wait()
	log.Info("All clients finished")
	endTime := time.Now().UTC()
	promTestEnd.WithLabelValues(testConfig.Name).Set(float64(endTime.UnixNano() / int64(1000000)))

	if testConfig.CleanAfter {
		log.Info("Housekeeping started")
		for _, work := range *Workqueue.Queue {
			err := work.Clean()
			if err != nil {
				log.WithError(err).Error("Error during cleanup - ignoring")
			}
		}
		for bucket := uint64(0); bucket < testConfig.Buckets.NumberMax; bucket++ {
			err := deleteBucket(housekeepingSvc, fmt.Sprintf("%s%s%d", workerID, testConfig.BucketPrefix, bucket))
			if err != nil {
				log.WithError(err).Error("Error during bucket deleting - ignoring")
			}
		}
		log.Info("Housekeeping finished")
	}
	// Sleep to ensure Prometheus can still scrape the last information before we restart the worker
	time.Sleep(10 * time.Second)
	return endTime.Sub(startTime)
}

func workUntilTimeout(Workqueue *Workqueue, workChannel chan WorkItem, notifyChan chan<- struct{}, runtime time.Duration) {
	timer := time.NewTimer(runtime)
	for {
		for _, work := range *Workqueue.Queue {
			select {
			case <-timer.C:
				log.Debug("Reached Runtime end")
				close(notifyChan)
				return
			case workChannel <- work:
			}
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case *DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				err := work.Prepare()
				if err != nil {
					log.WithError(err).Error("Error during work preparation - ignoring")
				}
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
					workChannel <- &Stopper{}
				}
				return
			}
			currentOps++
			workChannel <- work
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case *DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				err := work.Prepare()
				if err != nil {
					log.WithError(err).Error("Error during work preparation - ignoring")
				}
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func fillWorkqueue(testConfig *common.TestCaseConfiguration, Workqueue *Workqueue, workerID string, shareBucketName bool) {

	if testConfig.ReadWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "read"})
	}
	if testConfig.ExistingReadWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, KV{Key: "existing_read"})
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
		bucketName := fmt.Sprintf("%s%s%d", workerID, testConfig.BucketPrefix, bucket)
		if shareBucketName {
			bucketName = fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket)
		}
		err := createBucket(housekeepingSvc, bucketName)
		if err != nil {
			log.WithError(err).WithField("bucket", bucketName).Error("Error when creating bucket")
		}
		var preExistingObjects []types.Object
		var preExistingObjectCount uint64
		if testConfig.ExistingReadWeight > 0 {
			preExistingObjects, err = listObjects(housekeepingSvc, "", bucketName)
			if err != nil {
				log.WithError(err).Fatalf("Problems when listing contents of bucket %s", bucketName)
			}
			preExistingObjectCount = uint64(len(preExistingObjects))
			log.Debugf("Found %d objects in bucket %s", preExistingObjectCount, bucketName)

			if preExistingObjectCount <= 0 {
				log.Warningf("There is no objects in bucket %s", bucketName)
				continue
			}
		}
		objectCount := common.EvaluateDistribution(testConfig.Objects.NumberMin, testConfig.Objects.NumberMax, &testConfig.Objects.NumberLast, 1, testConfig.Objects.NumberDistribution)
		for object := uint64(0); object < objectCount; object++ {
			objectSize := common.EvaluateDistribution(testConfig.Objects.SizeMin, testConfig.Objects.SizeMax, &testConfig.Objects.SizeLast, 1, testConfig.Objects.SizeDistribution)

			nextOp := GetNextOperation(Workqueue)
			switch nextOp {
			case "read":
				err := IncreaseOperationValue(nextOp, 1/float64(testConfig.ReadWeight), Workqueue)
				if err != nil {
					log.WithError(err).Error("Could not increase operational Value - ignoring")
				}
				new := &ReadOperation{
					TestName:                 testConfig.Name,
					Bucket:                   bucketName,
					ObjectName:               fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize:               objectSize,
					WorksOnPreexistingObject: false,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "existing_read":
				err := IncreaseOperationValue(nextOp, 1/float64(testConfig.ExistingReadWeight), Workqueue)
				if err != nil {
					log.WithError(err).Error("Could not increase operational Value - ignoring")
				}
				new := &ReadOperation{
					TestName:                 testConfig.Name,
					Bucket:                   bucketName,
					ObjectName:               *preExistingObjects[object%preExistingObjectCount].Key,
					ObjectSize:               uint64(*preExistingObjects[object%preExistingObjectCount].Size),
					WorksOnPreexistingObject: true,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "write":
				err := IncreaseOperationValue(nextOp, 1/float64(testConfig.WriteWeight), Workqueue)
				if err != nil {
					log.WithError(err).Error("Could not increase operational Value - ignoring")
				}
				new := &WriteOperation{
					TestName:   testConfig.Name,
					Bucket:     bucketName,
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "list":
				err := IncreaseOperationValue(nextOp, 1/float64(testConfig.ListWeight), Workqueue)
				if err != nil {
					log.WithError(err).Error("Could not increase operational Value - ignoring")
				}
				new := &ListOperation{
					TestName:   testConfig.Name,
					Bucket:     bucketName,
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "delete":
				err := IncreaseOperationValue(nextOp, 1/float64(testConfig.DeleteWeight), Workqueue)
				if err != nil {
					log.WithError(err).Error("Could not increase operational Value - ignoring")
				}
				new := &DeleteOperation{
					TestName:   testConfig.Name,
					Bucket:     bucketName,
					ObjectName: fmt.Sprintf("%s%s%d", workerID, testConfig.ObjectPrefix, object),
					ObjectSize: objectSize,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			}
		}
	}
}
