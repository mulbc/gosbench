package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/lukechampine/fastxor"
	"github.com/mulbc/gosbench/common"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	rand.Seed(time.Now().UnixNano())
}

func main() {

}

func generateRandomBytes(size uint64) *[]byte {
	now := time.Now()
	random := make([]byte, size)
	n, err := rand.Read(random)
	if err != nil {
		log.Fatal("I had issues getting my random bytes initialized")
	}
	log.Tracef("Generated %d random bytes in %v", n, time.Since(now))
	return &random
}

// PerfTest runs a performance test as configured in testConfig
func PerfTest(testConfig *common.TestCaseConfiguration) {
	Workqueue := &common.Workqueue{
		Queue: &[]common.WorkItem{},
	}
	if testConfig.ReadWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, common.KV{Key: "read"})
	}
	if testConfig.WriteWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, common.KV{Key: "write"})
	}
	if testConfig.ListWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, common.KV{Key: "list"})
	}
	if testConfig.DeleteWeight > 0 {
		Workqueue.OperationValues = append(Workqueue.OperationValues, common.KV{Key: "delete"})
	}
	fillWorkqueue(testConfig, Workqueue)

	log.Info("Work preparation started")
	for _, work := range *Workqueue.Queue {
		work.Prepare()
	}
	log.Info("Work preparation finished")
	workChannel := make(chan common.WorkItem, len(*Workqueue.Queue))
	doneChannel := make(chan bool)
	for worker := 0; worker < testConfig.ParallelClients; worker++ {
		go common.DoWork(workChannel, doneChannel)
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

func workUntilTimeout(Workqueue *common.Workqueue, workChannel chan common.WorkItem, runtime time.Duration) {
	timer := time.NewTimer(runtime)
	for {
		for _, work := range *Workqueue.Queue {
			select {
			case <-timer.C:
				log.Debug("Reached Runtime end")
				common.WorkCancel()
				return
			case workChannel <- work:
			}
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case common.DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.Prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func workUntilOps(Workqueue *common.Workqueue, workChannel chan common.WorkItem, maxOps uint64, numberOfWorker int) {
	currentOps := uint64(0)
	for {
		for _, work := range *Workqueue.Queue {
			if currentOps >= maxOps {
				log.Debug("Reached OpsDeadline ... waiting for workers to finish")
				for worker := 0; worker < numberOfWorker; worker++ {
					workChannel <- common.Stopper{}
				}
				return
			}
			currentOps++
			workChannel <- work
		}
		for _, work := range *Workqueue.Queue {
			switch work.(type) {
			case common.DeleteOperation:
				log.Debug("Re-Running Work preparation for delete job started")
				work.Prepare()
				log.Debug("Delete preparation re-run finished")
			}
		}
	}
}

func fillWorkqueue(testConfig *common.TestCaseConfiguration, Workqueue *common.Workqueue) {

	random := make(map[uint64]*[]byte, testConfig.Objects.NumberMax)
	// Init random data (cannot be done in parallel)
	for object := uint64(0); object < testConfig.Objects.NumberMax; object++ {
		random[object] = generateRandomBytes(testConfig.Objects.SizeMax)
	}

	bucketCount := common.EvaluateDistribution(testConfig.Buckets.NumberMin, testConfig.Buckets.NumberMax, &testConfig.Buckets.NumberLast, 1, testConfig.Buckets.NumberDistribution)
	for bucket := uint64(0); bucket < bucketCount; bucket++ {
		objectCount := common.EvaluateDistribution(testConfig.Objects.NumberMin, testConfig.Objects.NumberMax, &testConfig.Objects.NumberLast, 1, testConfig.Objects.NumberDistribution)
		for object := uint64(0); object < objectCount; object++ {
			objectSize := common.EvaluateDistribution(testConfig.Objects.SizeMin, testConfig.Objects.SizeMax, &testConfig.Objects.SizeLast, 1, testConfig.Objects.SizeDistribution)
			objectContent := make([]byte, objectSize)

			// We reuse the same random []byte for every bucket
			// Use xor to make them different
			fastxor.Byte(objectContent, *random[object], byte(rand.Int()))

			nextOp := common.GetNextOperation(Workqueue)
			switch nextOp {
			case "read":
				common.IncreaseOperationValue(nextOp, 1/float64(testConfig.ReadWeight), Workqueue)
				new := common.ReadOperation{
					Bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					ObjectName:    fmt.Sprintf("%s%d", testConfig.ObjectPrefix, object),
					ObjectSize:    objectSize,
					ObjectContent: &objectContent,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "write":
				common.IncreaseOperationValue(nextOp, 1/float64(testConfig.WriteWeight), Workqueue)
				new := common.WriteOperation{
					Bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					ObjectName:    fmt.Sprintf("%s%d", testConfig.ObjectPrefix, object),
					ObjectSize:    objectSize,
					ObjectContent: &objectContent,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "list":
				common.IncreaseOperationValue(nextOp, 1/float64(testConfig.ListWeight), Workqueue)
				new := common.ListOperation{
					Bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					ObjectName:    fmt.Sprintf("%s%d", testConfig.ObjectPrefix, object),
					ObjectSize:    objectSize,
					ObjectContent: &objectContent,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			case "delete":
				common.IncreaseOperationValue(nextOp, 1/float64(testConfig.DeleteWeight), Workqueue)
				new := common.DeleteOperation{
					Bucket:        fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket),
					ObjectName:    fmt.Sprintf("%s%d", testConfig.ObjectPrefix, object),
					ObjectSize:    objectSize,
					ObjectContent: &objectContent,
				}
				*Workqueue.Queue = append(*Workqueue.Queue, new)
			}
		}
	}
}
