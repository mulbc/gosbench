package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// This uses the Base 2 calculation where
// 1 kB = 1024 Byte
const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
)

type s3Configuration struct {
	AccessKey string        `yaml:"access_key"`
	SecretKey string        `yaml:"secret_key"`
	Region    string        `yaml:"region"`
	Endpoint  string        `yaml:"endpoint"`
	Timeout   time.Duration `yaml:"timeout"`
}

type grafanaConfiguration struct {
	username string `yaml:"username"`
	password string `yaml:"password"`
	Endpoint string `yaml:"endpoint"`
}

type testCaseConfiguration struct {
	Objects struct {
		SizeMin            uint64 `yaml:"size_min"`
		SizeMax            uint64 `yaml:"size_max"`
		PartSize           uint64 `yaml:"part_size"`
		sizeLast           uint64
		SizeDistribution   string `yaml:"size_distribution"`
		NumberMin          uint64 `yaml:"number_min"`
		NumberMax          uint64 `yaml:"number_max"`
		numberLast         uint64
		NumberDistribution string `yaml:"number_distribution"`
		Unit               string `yaml:"unit"`
	} `yaml:"objects"`
	Buckets struct {
		NumberMin          uint64 `yaml:"number_min"`
		NumberMax          uint64 `yaml:"number_max"`
		numberLast         uint64
		NumberDistribution string `yaml:"number_distribution"`
	} `yaml:"buckets"`
	BucketPrefix    string        `yaml:"bucket_prefix"`
	Runtime         time.Duration `yaml:"stop_with_runtime"`
	OpsDeadline     uint64        `yaml:"stop_with_ops"`
	Workers         int           `yaml:"workers"`
	ParallelClients int           `yaml:"parallel_clients"`
	CleanAfter      bool          `yaml:"clean_after"`
	ReadWeight      int           `yaml:"read_weight"`
	WriteWeight     int           `yaml:"write_weight"`
	ListWeight      int           `yaml:"list_weight"`
	DeleteWeight    int           `yaml:"delete_weight"`
}

type testconf struct {
	S3Config      []*s3Configuration       `yaml:"s3_config"`
	GrafanaConfig *grafanaConfiguration    `yaml:"grafana_config"`
	Tests         []*testCaseConfiguration `yaml:"tests"`
}

var configFileLocation string
var config testconf

func init() {
	flag.StringVar(&configFileLocation, "c", "", "Config file describing test run")
	flag.Parse()
	if configFileLocation == "" {
		log.Fatal("-c is a mandatory parameter - please specify the config file")
	}

	configFileContent, err := ioutil.ReadFile(configFileLocation)
	if err != nil {
		log.WithError(err).Fatalf("Error reading config file:")
	}
	err = yaml.Unmarshal(configFileContent, &config)
	if err != nil {
		log.WithError(err).Fatalf("Error unmarshaling config file:")
	}
}

func checkConfig() {
	for _, testcase := range config.Tests {
		// log.Debugf("Checking testcase with prefix %s", testcase.BucketPrefix)
		err := checkTestCase(testcase)
		if err != nil {
			log.WithError(err).Fatalf("Issue detected when scanning through the config file:")
		}
	}
}

func checkTestCase(testcase *testCaseConfiguration) error {
	if testcase.Runtime == 0 && testcase.OpsDeadline == 0 {
		return fmt.Errorf("Either stop_with_runtime or stop_with_ops needs to be set")
	}
	if testcase.ReadWeight == 0 && testcase.WriteWeight == 0 && testcase.ListWeight == 0 && testcase.DeleteWeight == 0 {
		return fmt.Errorf("At least one weight needs to be set - Read / Write / List / Delete")
	}
	if testcase.Buckets.NumberMin == 0 {
		return fmt.Errorf("Please set minimum number of Buckets")
	}
	if testcase.Objects.SizeMin == 0 {
		return fmt.Errorf("Please set minimum size of Objects")
	}
	if testcase.Objects.SizeMax == 0 {
		return fmt.Errorf("Please set maximum size of Objects")
	}
	if testcase.Objects.NumberMin == 0 {
		return fmt.Errorf("Please set minimum number of Objects")
	}
	if err := checkDistribution(testcase.Objects.SizeDistribution, "Object size_distribution"); err != nil {
		return err
	}
	if err := checkDistribution(testcase.Objects.NumberDistribution, "Object number_distribution"); err != nil {
		return err
	}
	if err := checkDistribution(testcase.Buckets.NumberDistribution, "Bucket number_distribution"); err != nil {
		return err
	}
	if testcase.Objects.Unit == "" {
		return fmt.Errorf("Please set the Objects unit")
	}

	var toByteMultiplicator uint64
	switch strings.ToUpper(testcase.Objects.Unit) {
	case "B":
		toByteMultiplicator = BYTE
	case "KB", "K":
		toByteMultiplicator = KILOBYTE
	case "MB", "M":
		toByteMultiplicator = MEGABYTE
	case "GB", "G":
		toByteMultiplicator = GIGABYTE
	case "TB", "T":
		toByteMultiplicator = TERABYTE
	default:
		return fmt.Errorf("Could not parse unit size - please use one of B/KB/MB/GB/TB")
	}

	testcase.Objects.SizeMin = testcase.Objects.SizeMin * toByteMultiplicator
	testcase.Objects.SizeMax = testcase.Objects.SizeMax * toByteMultiplicator
	testcase.Objects.PartSize = testcase.Objects.PartSize * toByteMultiplicator
	return nil
}

// Checks if a given string is of type distribution
func checkDistribution(distribution string, keyname string) error {
	switch distribution {
	case "constant", "random", "sequential":
		return nil
	}
	return fmt.Errorf("%s is not a valid distribution. Allowed options are constant, random, sequential", keyname)
}

func evaluateDistribution(min uint64, max uint64, lastNumber *uint64, increment uint64, distribution string) uint64 {
	switch distribution {
	case "constant":
		return min
	case "random":
		rand.Seed(time.Now().UnixNano())
		validSize := max - min
		return ((rand.Uint64() % validSize) + min)
	case "sequential":
		if *lastNumber+increment > max {
			return max
		}
		*lastNumber = *lastNumber + increment
		return *lastNumber
	}
	return 0
}
