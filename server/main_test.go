package main

import (
	"reflect"
	"testing"

	"github.com/mulbc/gosbench/common"
)

func Test_loadConfigFromFile(t *testing.T) {
	type args struct {
		configFileContent []byte
	}
	tests := []struct {
		name string
		args args
		want common.Testconf
	}{
		{"empty file", args{[]byte{}}, common.Testconf{}},
		// TODO discover how to handle log.Fatal with logrus here
		// https://github.com/sirupsen/logrus#fatal-handlers
		// {"unparsable", args{[]byte(`corrupt!`)}, common.Testconf{}},
		{"S3Config", args{[]byte(`s3_config:
  - access_key: secretKey
    secret_key: secretSecret
    endpoint: test`)}, common.Testconf{
			S3Config: []*common.S3Configuration{&common.S3Configuration{
				Endpoint:  "test",
				AccessKey: "secretKey",
				SecretKey: "secretSecret",
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Log("Recovered in f", r)
				}
			}()
			if got := loadConfigFromFile(tt.args.configFileContent); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadConfigFromFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
