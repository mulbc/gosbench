package main

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
)

type workItem interface {
	prepare() error
	do() error
	clean() error
}

type readOperation struct {
	bucket        string
	objectName    string
	objectSize    uint64
	objectContent *[]byte
}
type writeOperation struct {
	bucket        string
	objectName    string
	objectSize    uint64
	objectContent *[]byte
}
type listOperation struct {
	bucket        string
	objectName    string
	objectSize    uint64
	objectContent *[]byte
}
type deleteOperation struct {
	bucket        string
	objectName    string
	objectSize    uint64
	objectContent *[]byte
}

type stopper struct{}

type kv struct {
	Key   string
	Value float64
}

type workqueue struct {
	operationValues []kv
	queue           *[]workItem
}

func getNextOperation(queue *workqueue) string {
	sort.Slice(queue.operationValues, func(i, j int) bool {
		return queue.operationValues[i].Value < queue.operationValues[j].Value
	})
	return queue.operationValues[0].Key
}

func init() {
	workContext, workCancel = context.WithCancel(context.Background())
}

var workContext context.Context
var workCancel context.CancelFunc

func increaseOperationValue(operation string, value float64, queue *workqueue) error {
	log.Debug("Tests")
	for i := range queue.operationValues {
		if queue.operationValues[i].Key == operation {
			queue.operationValues[i].Value += value
			return nil
		}
	}
	return fmt.Errorf("Could not find requested operation %s", operation)
}

func (op readOperation) prepare() error {
	err := createBucket(housekeepingSvc, op.bucket)
	if err != nil {
		return err
	}
	return putObject(housekeepingSvc, op.objectName, bytes.NewReader(*op.objectContent), op.bucket)
}
func (op writeOperation) prepare() error {
	return createBucket(housekeepingSvc, op.bucket)
}
func (op listOperation) prepare() error {
	err := createBucket(housekeepingSvc, op.bucket)
	if err != nil {
		return err
	}
	return putObject(housekeepingSvc, op.objectName, bytes.NewReader(*op.objectContent), op.bucket)
}
func (op deleteOperation) prepare() error {
	err := createBucket(housekeepingSvc, op.bucket)
	if err != nil {
		return err
	}
	return deleteObject(housekeepingSvc, op.objectName, op.bucket)
}
func (op stopper) prepare() error {
	return nil
}

func (op readOperation) do() error {
	log.Debug("Doing readOperation")
	return getObject(svc, op.objectName, op.bucket)
}
func (op writeOperation) do() error {
	log.Debug("Doing writeOperation")
	return putObject(svc, op.objectName, bytes.NewReader(*op.objectContent), op.bucket)
}
func (op listOperation) do() error {
	log.Debug("Doing listOperation")
	return listObjects(svc, op.objectName, op.bucket)
}
func (op deleteOperation) do() error {
	log.Debug("Doing deleteOperation")
	return deleteObject(svc, op.objectName, op.bucket)
}
func (op stopper) do() error {
	return nil
}

func (op readOperation) clean() error {
	err := deleteObject(housekeepingSvc, op.objectName, op.bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.bucket)
}
func (op writeOperation) clean() error {
	err := deleteObject(housekeepingSvc, op.objectName, op.bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.bucket)
}
func (op listOperation) clean() error {
	err := deleteObject(housekeepingSvc, op.objectName, op.bucket)
	if err != nil {
		return err
	}
	return deleteBucket(housekeepingSvc, op.bucket)
}
func (op deleteOperation) clean() error {
	return deleteBucket(housekeepingSvc, op.bucket)
}
func (op stopper) clean() error {
	return nil
}

func doWork(workChannel chan workItem, doneChannel chan bool) {
	for {
		select {
		case <-workContext.Done():
			log.Debugf("Runtime over - Got timeout from work context")
			doneChannel <- true
			return
		case work := <-workChannel:
			switch work.(type) {
			case stopper:
				log.Debug("Found the end of the work queue - stopping")
				doneChannel <- true
				return
			}
			work.do()
		}
	}
}
