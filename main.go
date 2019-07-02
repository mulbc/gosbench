package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/lukechampine/fastxor"
	log "github.com/sirupsen/logrus"
)

// APPVERSION displays this App's version which will
// also be available as label in the Prometheus exporter
const APPVERSION = "0.0.1"

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// TODO:
	//  * Init gRPC
	//  * Create Grafana annotations when starting/stopping testcase
	//  *
	//

	checkConfig()
	initS3()
	for _, test := range config.Tests {
		perfTest(test)
	}
	log.Info("Sleeping for 60s")
	time.Sleep(time.Second * 60)
}

func generateRandomBytes(size uint64) *[]byte {
	now := time.Now()
	random := make([]byte, size)
	n, err := rand.Read(random)
	if err != nil {
		log.Fatal("I had issues getting my random bytes initialized")
	}
	log.Debugf("Generated %d random bytes in %v", n, time.Since(now))
	return &random
}

func perfTest(testConfig *testCaseConfiguration) {
	workQueue := &workqueue{
		queue: &[]workItem{},
	}
	if testConfig.ReadWeight > 0 {
		workQueue.operationValues = append(workQueue.operationValues, kv{Key: "read"})
	}
	if testConfig.WriteWeight > 0 {
		workQueue.operationValues = append(workQueue.operationValues, kv{Key: "write"})
	}
	if testConfig.ListWeight > 0 {
		workQueue.operationValues = append(workQueue.operationValues, kv{Key: "list"})
	}
	if testConfig.DeleteWeight > 0 {
		workQueue.operationValues = append(workQueue.operationValues, kv{Key: "delete"})
	}
	fillWorkQueue(testConfig, workQueue)

	for _, work := range *workQueue.queue {
		log.Debug("Work preparation started")
		work.prepare()
		log.Debug("Work preparation finished")
	}
	workChannel := make(chan workItem, len(*workQueue.queue))
	doneChannel := make(chan bool)
	for worker := 0; worker < testConfig.ParallelClients; worker++ {
		go doWork(workChannel, doneChannel)
	}
	if testConfig.Runtime != 0 {
		workUntilTimeout(workQueue, workChannel, testConfig.Runtime)
	} else {
		workUntilOps(workQueue, workChannel, testConfig.OpsDeadline, testConfig.ParallelClients)
	}
	// Wait for all the goroutines to finish
	for i := 0; i < testConfig.ParallelClients; i++ {
		<-doneChannel
	}
	if testConfig.CleanAfter {
		log.Debug("Housekeeping started")
		for _, work := range *workQueue.queue {
			work.clean()
		}
		log.Debug("Housekeeping finished")
	}
}

func workUntilTimeout(workQueue *workqueue, workChannel chan workItem, runtime time.Duration) {
	timer := time.NewTimer(runtime)
	for {
		for _, work := range *workQueue.queue {
			select {
			case <-timer.C:
				log.Debug("Reached Runtime end")
				workCancel()
				return
			case workChannel <- work:
			}
		}
		for _, work := range *workQueue.queue {
			switch work.(type) {
			case deleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func workUntilOps(workQueue *workqueue, workChannel chan workItem, maxOps uint64, numberOfWorker int) {
	currentOps := uint64(0)
	for {
		for _, work := range *workQueue.queue {
			if currentOps >= maxOps {
				log.Debug("We've added enough Ops to our queue... waiting for workers to finish this")
				for worker := 0; worker < numberOfWorker; worker++ {
					workChannel <- stopper{}
				}
				return
			}
			currentOps++
			workChannel <- work
		}
		for _, work := range *workQueue.queue {
			switch work.(type) {
			case deleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func fillWorkQueue(testConfig *testCaseConfiguration, workQueue *workqueue) {

	random := make(map[uint64]*[]byte, testConfig.Objects.NumberMax)
	// Init random data (cannot be done in parallel)
	for object := uint64(0); object < testConfig.Objects.NumberMax; object++ {
		random[object] = generateRandomBytes(testConfig.Objects.SizeMax)
	}

	bucketCount := evaluateDistribution(testConfig.Buckets.NumberMin, testConfig.Buckets.NumberMax, &testConfig.Buckets.numberLast, 1, testConfig.Buckets.NumberDistribution)
	for bucket := uint64(0); bucket < bucketCount; bucket++ {
		objectCount := evaluateDistribution(testConfig.Objects.NumberMin, testConfig.Objects.NumberMax, &testConfig.Objects.numberLast, 1, testConfig.Objects.NumberDistribution)
		for object := uint64(0); object < objectCount; object++ {
			objectSize := evaluateDistribution(testConfig.Objects.SizeMin, testConfig.Objects.SizeMax, &testConfig.Objects.sizeLast, 1, testConfig.Objects.SizeDistribution)
			objectContent := make([]byte, objectSize)

			// We reuse the same random []byte for every bucket
			// Use xor to make them different
			fastxor.Byte(objectContent, *random[object], byte(rand.Int()))

			nextOp := getNextOperation(workQueue)
			switch nextOp {
			case "read":
				increaseOperationValue(nextOp, 1/float64(testConfig.ReadWeight), workQueue)
				new := readOperation{
					bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					objectName:    fmt.Sprintf("obj%d", object),
					objectSize:    objectSize,
					objectContent: &objectContent,
				}
				*workQueue.queue = append(*workQueue.queue, new)
			case "write":
				increaseOperationValue(nextOp, 1/float64(testConfig.WriteWeight), workQueue)
				new := writeOperation{
					bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					objectName:    fmt.Sprintf("obj%d", object),
					objectSize:    objectSize,
					objectContent: &objectContent,
				}
				*workQueue.queue = append(*workQueue.queue, new)
			case "list":
				increaseOperationValue(nextOp, 1/float64(testConfig.ListWeight), workQueue)
				new := listOperation{
					bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					objectName:    fmt.Sprintf("obj%d", object),
					objectSize:    objectSize,
					objectContent: &objectContent,
				}
				*workQueue.queue = append(*workQueue.queue, new)
			case "delete":
				increaseOperationValue(nextOp, 1/float64(testConfig.DeleteWeight), workQueue)
				new := deleteOperation{
					bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					objectName:    fmt.Sprintf("obj%d", object),
					objectSize:    objectSize,
					objectContent: &objectContent,
				}
				*workQueue.queue = append(*workQueue.queue, new)
			}
		}
	}
}
