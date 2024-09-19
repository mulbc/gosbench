package common

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func Test_checkTestCase(t *testing.T) {
	type args struct {
		testcase *TestCaseConfiguration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"No end defined", args{new(TestCaseConfiguration)}, true},
		{"No weights defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10}}, true},
		{"No Bucket Numbers defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1}}, true},
		{"No Object size min defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			}}}, true},
		{"No Object size max defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin: 1,
			}}}, true},
		{"No Object number min defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin: 1,
				SizeMax: 2,
			}}}, true},
		{"No Object size distributions defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:   1,
				SizeMax:   2,
				NumberMin: 3,
			}}}, true},
		{"No Object number distributions defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:          1,
				SizeMax:          2,
				NumberMin:        3,
				SizeDistribution: "constant",
			}}}, true},
		{"No Bucket distribution defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin: 1,
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:            1,
				SizeMax:            2,
				NumberMin:          3,
				SizeDistribution:   "constant",
				NumberDistribution: "constant",
			}}}, true},
		{"No Object Unit defined", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin:          1,
				NumberDistribution: "constant",
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:            1,
				SizeMax:            2,
				NumberMin:          3,
				SizeDistribution:   "constant",
				NumberDistribution: "constant",
			}}}, true},
		{"Wrong object unit", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin:          1,
				NumberDistribution: "constant",
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:            1,
				SizeMax:            2,
				NumberMin:          3,
				SizeDistribution:   "constant",
				NumberDistribution: "constant",
				Unit:               "XB",
			}}}, true},
		{"Existing object read without bucket prefix", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ExistingReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin:          1,
				NumberDistribution: "constant",
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:            1,
				SizeMax:            2,
				NumberMin:          3,
				SizeDistribution:   "constant",
				NumberDistribution: "constant",
				Unit:               "XB",
			}}}, true},
		{"All good", args{&TestCaseConfiguration{Runtime: Duration(time.Second), OpsDeadline: 10, ReadWeight: 1,
			Buckets: struct {
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
			}{
				NumberMin:          1,
				NumberDistribution: "constant",
			},
			Objects: struct {
				SizeMin            uint64 `yaml:"size_min" json:"size_min"`
				SizeMax            uint64 `yaml:"size_max" json:"size_max"`
				PartSize           uint64 `yaml:"part_size" json:"part_size"`
				SizeLast           uint64
				SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
				NumberMin          uint64 `yaml:"number_min" json:"number_min"`
				NumberMax          uint64 `yaml:"number_max" json:"number_max"`
				NumberLast         uint64
				NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
				Unit               string `yaml:"unit" json:"unit"`
			}{
				SizeMin:            1,
				SizeMax:            2,
				NumberMin:          3,
				SizeDistribution:   "constant",
				NumberDistribution: "constant",
				Unit:               "KB",
			}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkTestCase(tt.args.testcase); (err != nil) != tt.wantErr {
				t.Errorf("checkTestCase() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_checkDistribution(t *testing.T) {
	type args struct {
		distribution string
		keyname      string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"constant distribution", args{"constant", "test"}, false},
		{"random distribution", args{"random", "test"}, false},
		{"sequential distribution", args{"sequential", "test"}, false},
		{"wrong distribution", args{"wrong", "test"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkDistribution(tt.args.distribution, tt.args.keyname); (err != nil) != tt.wantErr {
				t.Errorf("checkDistribution() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateDistribution(t *testing.T) {
	type args struct {
		min          uint64
		max          uint64
		lastNumber   *uint64
		increment    uint64
		distribution string
	}
	lastArgumentNumber := uint64(1)
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"constant distribution", args{5, 100, &lastArgumentNumber, 1, "constant"}, 5},
		{"random distribution", args{1, 2, &lastArgumentNumber, 1, "random"}, 1},
		{"sequential distribution", args{1, 10, &lastArgumentNumber, 1, "sequential"}, 2},
		{"last number in sequential distribution", args{1, 10, &lastArgumentNumber, 10, "sequential"}, 10},
		{"wrong distribution", args{1, 10, &lastArgumentNumber, 1, "wrong"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateDistribution(tt.args.min, tt.args.max, tt.args.lastNumber, tt.args.increment, tt.args.distribution); got != tt.want {
				t.Errorf("EvaluateDistribution() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_loadConfigFromFile(t *testing.T) {
	read := func(content []byte) func(string) ([]byte, error) {
		return func(string) ([]byte, error) {
			return content, nil
		}
	}
	defer func() {
		ReadFile = os.ReadFile
	}()
	type args struct {
		configFileContent []byte
	}
	tests := []struct {
		name string
		args args
		want *Testconf
	}{
		{"empty file", args{[]byte{}}, &Testconf{}},
		// TODO discover how to handle log.Fatal with logrus here
		// https://github.com/sirupsen/logrus#fatal-handlers
		// {"unparsable", args{[]byte(`corrupt!`)}, common.Testconf{}},
		{"S3Config", args{[]byte(`s3_config:
  - access_key: secretKey
    secret_key: secretSecret
    region: us-east-1
    endpoint: http://10.9.8.72:80
    skipSSLverify: true
tests:
  - name: clean-4k
    delete_weight: 100
    objects:
      size_min: 4
      size_max: 4
      size_distribution: constant
      unit: KB
      number_min: 1
      number_max: 100000
      number_distribution: sequential
    buckets:
      number_min: 1
      number_max: 10
      number_distribution: sequential
    bucket_prefix: gosbench-prefix-
    object_prefix: obj-
    stop_with_ops: 10
    stop_with_runtime: 36000s # Example with 60 seconds runtime
    workers: 3
    workers_share_buckets: False
    parallel_clients: 3
    clean_after: True
`)}, &Testconf{
			S3Config: []*S3Configuration{
				{
					Endpoint:      "http://10.9.8.72:80",
					AccessKey:     "secretKey",
					SecretKey:     "secretSecret",
					Region:        "us-east-1",
					SkipSSLVerify: true,
					UsePathStyle:  false,
				},
			},
			Tests: []*TestCaseConfiguration{
				{
					Name:               "clean-4k",
					DeleteWeight:       100,
					BucketPrefix:       "gosbench-prefix-",
					ObjectPrefix:       "obj-",
					Runtime:            Duration(36000 * time.Second),
					OpsDeadline:        10,
					Workers:            3,
					WorkerShareBuckets: false,
					ParallelClients:    3,
					CleanAfter:         true,
					Buckets: struct {
						NumberMin          uint64 `yaml:"number_min" json:"number_min"`
						NumberMax          uint64 `yaml:"number_max" json:"number_max"`
						NumberLast         uint64
						NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
					}{
						NumberMin:          1,
						NumberMax:          10,
						NumberDistribution: "sequential",
					},
					Objects: struct {
						SizeMin            uint64 `yaml:"size_min" json:"size_min"`
						SizeMax            uint64 `yaml:"size_max" json:"size_max"`
						PartSize           uint64 `yaml:"part_size" json:"part_size"`
						SizeLast           uint64
						SizeDistribution   string `yaml:"size_distribution" json:"size_distribution"`
						NumberMin          uint64 `yaml:"number_min" json:"number_min"`
						NumberMax          uint64 `yaml:"number_max" json:"number_max"`
						NumberLast         uint64
						NumberDistribution string `yaml:"number_distribution" json:"number_distribution"`
						Unit               string `yaml:"unit" json:"unit"`
					}{
						SizeMin:            4,
						SizeMax:            4,
						SizeDistribution:   "constant",
						Unit:               "KB",
						NumberMin:          1,
						NumberMax:          100000,
						NumberDistribution: "sequential",
					},
				},
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Log("Recovered in f", r)
				}
			}()
			ReadFile = read(tt.args.configFileContent)
			if got := LoadConfigFromFile("configFile.yaml"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadConfigFromFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
