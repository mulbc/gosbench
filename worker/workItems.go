package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
)

// WorkItem is an interface for general work operations
// They can be read,write,list,delete or a stopper
type WorkItem interface {
	Prepare() error
	Do() error
	Clean() error
}

// ReadOperation stands for a read operation
type ReadOperation struct {
	Bucket     string
	ObjectName string
	ObjectSize uint64
}

// WriteOperation stands for a write operation
type WriteOperation struct {
	Bucket     string
	ObjectName string
	ObjectSize uint64
}

// ListOperation stands for a list operation
type ListOperation struct {
	Bucket     string
	ObjectName string
	ObjectSize uint64
}

// DeleteOperation stands for a delete operation
type DeleteOperation struct {
	Bucket     string
	ObjectName string
	ObjectSize uint64
}

// Stopper marks the end of a workqueue when using
// maxOps as testCase end criterium
type Stopper struct{}

// KV is a simple key-value struct
type KV struct {
	Key   string
	Value float64
}

// Workqueue contains the Queue and the valid operation's
// values to determine which operation should be done next
// in order to satisfy the set ratios.
type Workqueue struct {
	OperationValues []KV
	Queue           *[]WorkItem
}

// GetNextOperation evaluates the operation values and returns which
// operation should happen next
func GetNextOperation(Queue *Workqueue) string {
	sort.Slice(Queue.OperationValues, func(i, j int) bool {
		return Queue.OperationValues[i].Value < Queue.OperationValues[j].Value
	})
	return Queue.OperationValues[0].Key
}

func init() {
	workContext, WorkCancel = context.WithCancel(context.Background())
}

var workContext context.Context

// WorkCancel is the function to stop the execution of jobs
var WorkCancel context.CancelFunc

// IncreaseOperationValue increases the given operation's value by the set amount
func IncreaseOperationValue(operation string, value float64, Queue *Workqueue) error {
	for i := range Queue.OperationValues {
		if Queue.OperationValues[i].Key == operation {
			Queue.OperationValues[i].Value += value
			return nil
		}
	}
	return fmt.Errorf("Could not find requested operation %s", operation)
}

// Prepare prepares the execution of the ReadOperation
func (op ReadOperation) Prepare() error {
	log.WithField("bucket", op.Bucket).WithField("object", op.ObjectName).Debug("Preparing ReadOperation")
	return putObject(housekeepingSvc, op.ObjectName, bytes.NewReader(generateRandomBytes(op.ObjectSize)), op.Bucket)
}

// Prepare prepares the execution of the WriteOperation
func (op WriteOperation) Prepare() error {
	log.WithField("bucket", op.Bucket).WithField("object", op.ObjectName).Debug("Preparing WriteOperation")
	return nil
}

// Prepare prepares the execution of the ListOperation
func (op ListOperation) Prepare() error {
	log.WithField("bucket", op.Bucket).WithField("object", op.ObjectName).Debug("Preparing ListOperation")
	return putObject(housekeepingSvc, op.ObjectName, bytes.NewReader(generateRandomBytes(op.ObjectSize)), op.Bucket)
}

// Prepare prepares the execution of the DeleteOperation
func (op DeleteOperation) Prepare() error {
	log.WithField("bucket", op.Bucket).WithField("object", op.ObjectName).Debug("Preparing DeleteOperation")
	return putObject(housekeepingSvc, op.ObjectName, bytes.NewReader(generateRandomBytes(op.ObjectSize)), op.Bucket)
}

// Prepare does nothing here
func (op Stopper) Prepare() error {
	return nil
}

// Do executes the actual work of the ReadOperation
func (op ReadOperation) Do() error {
	log.Debug("Doing ReadOperation")
	return getObject(svc, op.ObjectName, op.Bucket)
}

// Do executes the actual work of the WriteOperation
func (op WriteOperation) Do() error {
	log.Debug("Doing WriteOperation")
	return putObject(svc, op.ObjectName, bytes.NewReader(generateRandomBytes(op.ObjectSize)), op.Bucket)
}

// Do executes the actual work of the ListOperation
func (op ListOperation) Do() error {
	log.Debug("Doing ListOperation")
	return listObjects(svc, op.ObjectName, op.Bucket)
}

// Do executes the actual work of the DeleteOperation
func (op DeleteOperation) Do() error {
	log.Debug("Doing DeleteOperation")
	return deleteObject(svc, op.ObjectName, op.Bucket)
}

// Do does nothing here
func (op Stopper) Do() error {
	return nil
}

// Clean removes the objects and buckets left from the previous ReadOperation
func (op ReadOperation) Clean() error {
	err := deleteObject(housekeepingSvc, op.ObjectName, op.Bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.Bucket)
}

// Clean removes the objects and buckets left from the previous WriteOperation
func (op WriteOperation) Clean() error {
	err := deleteObject(housekeepingSvc, op.ObjectName, op.Bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.Bucket)
}

// Clean removes the objects and buckets left from the previous ListOperation
func (op ListOperation) Clean() error {
	err := deleteObject(housekeepingSvc, op.ObjectName, op.Bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.Bucket)
}

// Clean removes the objects and buckets left from the previous DeleteOperation
func (op DeleteOperation) Clean() error {
	return deleteBucket(housekeepingSvc, op.Bucket)
}

// Clean does nothing here
func (op Stopper) Clean() error {
	return nil
}

// DoWork processes the workitems in the workChannel until
// either the time runs out or a stopper is found
func DoWork(workChannel chan WorkItem, doneChannel chan bool) {
	for {
		select {
		case <-workContext.Done():
			log.Debugf("Runtime over - Got timeout from work context")
			doneChannel <- true
			return
		case work := <-workChannel:
			switch work.(type) {
			case Stopper:
				log.Debug("Found the end of the work Queue - stopping")
				doneChannel <- true
				return
			}
			work.Do()
		}
	}
}

func generateRandomBytes(size uint64) []byte {
	now := time.Now()
	random := make([]byte, size)
	n, err := rand.Read(random)
	if err != nil {
		log.WithError(err).Fatal("I had issues getting my random bytes initialized")
	}
	log.Tracef("Generated %d random bytes in %v", n, time.Since(now))
	return random
}
