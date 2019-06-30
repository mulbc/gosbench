package main

import (
	"bytes"
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
	if true || rand.Float64() >= testConfig.ReadRatio {
		random := make(map[uint64]*[]byte, testConfig.Objects.NumberMax)
		// Init random data (cannot be done in parallel)
		for object := uint64(0); object < testConfig.Objects.NumberMax; object++ {
			random[object] = generateRandomBytes(testConfig.Objects.SizeMax)
		}
		bucketCount := evaluateDistribution(testConfig.Buckets.NumberMin, testConfig.Buckets.NumberMax, &testConfig.Buckets.numberLast, 1, testConfig.Buckets.NumberDistribution)
		for bucket := uint64(0); bucket < bucketCount; bucket++ {
			err := createBucket(svc, fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket))
			if err != nil {
				log.WithError(err).Fatalf("Could not create bucket %s%d", testConfig.BucketPrefix, bucket)
			}
			objectCount := evaluateDistribution(testConfig.Objects.NumberMin, testConfig.Objects.NumberMax, &testConfig.Objects.numberLast, 1, testConfig.Objects.NumberDistribution)
			for object := uint64(0); object < objectCount; object++ {
				objectSize := evaluateDistribution(testConfig.Objects.SizeMin, testConfig.Objects.SizeMax, &testConfig.Objects.sizeLast, 1, testConfig.Objects.SizeDistribution)
				objectContent := make([]byte, objectSize)
				fastxor.Byte(objectContent, *random[object], byte(rand.Int()))
				putObject(svc, testConfig, fmt.Sprintf("obj%d", object), bytes.NewReader(objectContent), fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket))
			}
		}
	}
	if testConfig.CleanAfter {
		log.Debug("Housekeeping started")
		for bucket := uint64(0); bucket < testConfig.Buckets.NumberMax; bucket++ {
			// Ignore errors since we might not have created NumberMax buckets...
			// TODO: Only ignore errors for when the bucket did not yet exist...
			_ = deleteBucket(housekeepingSvc, fmt.Sprintf("%s%d", testConfig.BucketPrefix, bucket))
			// if aerr, ok := err.(awserr.Error); ok {
			// 	log.Debugf("THis is code %v", aerr.Code())
			// 	log.WithError(aerr).Errorf("I had trouble deleting bucket %s%d", testConfig.BucketPrefix, bucket)
			// }
		}
		log.Debug("Housekeeping finished")
	}
}
