package main

import (
	"context"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"

	"contrib.go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
)

var svc, housekeepingSvc *s3.S3
var ctx context.Context
var bucket string

// Uploads a file to S3 given a bucket and object key. Also takes a duration
// value to terminate the update if it doesn't complete within that time.
//
// The AWS Region needs to be provided in the AWS shared config or on the
// environment variable as `AWS_REGION`. Credentials also must be provided
// Will default to shared config file, but can load from environment if provided.
//
// Usage:
//   # Upload myfile.txt to myBucket/myKey. Must complete within 10 minutes or will fail
//   go run withContext.go -b mybucket -k myKey -d 10m < myfile.txt
func initS3() {
	// var timeout time.Duration

	// Then create the prometheus stat exporter
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "gosbench",
		ConstLabels: map[string]string{
			"version": APPVERSION,
		},
	})
	if err != nil {
		log.WithError(err).Fatalf("Failed to create the Prometheus exporter:")
	}

	// All clients require a Session. The Session provides the client with
	// shared configuration such as region, endpoint, and credentials. A
	// Session should be shared where possible to take advantage of
	// configuration and credential caching. See the session package for
	// more information.
	hc := &http.Client{Transport: new(ochttp.Transport)}

	sess := session.Must(session.NewSession(&aws.Config{
		HTTPClient: hc,
		Region:     &config.S3Config.Region,
	}))
	housekeepingSess := session.Must(session.NewSession(&aws.Config{
		Region: &config.S3Config.Region,
	}))

	if err := view.Register([]*view.View{
		ochttp.ClientSentBytesDistribution,
		ochttp.ClientReceivedBytesDistribution,
		ochttp.ClientRoundtripLatencyDistribution,
		ochttp.ClientCompletedCount,
	}...); err != nil {
		log.WithError(err).Fatalf("Failed to register HTTP client views:")
	}
	view.RegisterExporter(pe)
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", pe)
		// http://localhost:8888/metrics
		if err := http.ListenAndServe(":8888", mux); err != nil {
			log.WithError(err).Fatalf("Failed to run Prometheus /metrics endpoint:")
		}
	}()

	// Create a new instance of the service's client with a Session.
	// Optional aws.Config values can also be provided as variadic arguments
	// to the New function. This option allows you to provide service
	// specific configuration.
	svc = s3.New(sess)
	housekeepingSvc = s3.New(housekeepingSess)

	// Create a context with a timeout that will abort the data transfer if it takes
	// more than the passed in timeout.
	ctx = context.Background()
	var cancelFn func()
	if config.S3Config.Timeout > 0 {
		// ctx, cancelFn = context.WithTimeout(ctx, config.S3Config.Timeout)
	}
	// Ensure the context is canceled to prevent leaking.
	// See context package for more information, https://golang.org/pkg/context/
	if cancelFn != nil {
		defer cancelFn()
	}
	log.Debug("S3 Init done")
}

func putObject(service *s3.S3, testConfig *testCaseConfiguration, objectName string, objectContent io.ReadSeeker, bucket string) error {
	// Create an uploader with S3 client and custom options
	uploader := s3manager.NewUploaderWithClient(service, func(u *s3manager.Uploader) {
		u.PartSize = int64(testConfig.Objects.PartSize)
		u.Concurrency = testConfig.ParallelClients
	})

	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &bucket,
		Key:    &objectName,
		Body:   objectContent,
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			// If the SDK can determine the request or retry delay was canceled
			// by a context the CanceledErrorCode error code will be returned.
			log.WithError(aerr).Errorf("Upload canceled due to timeout")
		} else {
			log.WithError(err).Errorf("Failed to upload object,")
		}
		return err
	}

	log.WithField("bucket", bucket).WithField("key", objectName).Debugf("Upload successful")
	return err
}

func getObjectProperties(service *s3.S3, objectName string, bucket string) {
	result, err := svc.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
	})
	if err != nil {
		// Cast err to awserr.Error to handle specific error codes.
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == s3.ErrCodeNoSuchKey {
			log.WithError(aerr).Errorf("Could not find object %s in bucket %s when querying properties", objectName, bucket)
		}
	}

	// Make sure to close the body when done with it for S3 GetObject APIs or
	// will leak connections.
	defer result.Body.Close()

	log.Debugf("Object Properties:\n%+v", result)
}

func getObject(service *s3.S3, testConfig *testCaseConfiguration, objectName string, bucket string) error {
	// Create a downloader with the session and custom options
	downloader := s3manager.NewDownloaderWithClient(service, func(d *s3manager.Downloader) {
		d.PartSize = int64(testConfig.Objects.PartSize)
		d.Concurrency = testConfig.ParallelClients
	})
	buf := aws.NewWriteAtBuffer([]byte{})
	_, err := downloader.DownloadWithContext(ctx, buf, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
	})
	return err
}

func deleteObject(service *s3.S3, objectName string, bucket string) error {
	_, err := svc.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: &bucket,
		// Key:    objectName,
		Delete: &s3.Delete{
			Objects: []*s3.ObjectIdentifier{&s3.ObjectIdentifier{Key: &objectName}},
		},
	})
	if err != nil {
		// Cast err to awserr.Error to handle specific error codes.
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == s3.ErrCodeNoSuchKey {
			log.WithError(aerr).Errorf("Could not find object %s in bucket %s for deletion", objectName, bucket)
		}
		return err
	}

	log.Infof("Object %s/%s deleted\n", bucket, objectName)
	return err
}

func createBucket(service *s3.S3, bucket string) error {
	_, err := service.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bucket,
	})
	return err
}

func deleteBucket(service *s3.S3, bucket string) error {
	// First delete all objects in the bucket
	iter := s3manager.NewDeleteListIterator(service, &s3.ListObjectsInput{
		Bucket: &bucket,
	})

	if err := s3manager.NewBatchDeleteWithClient(service).Delete(aws.BackgroundContext(), iter); err != nil {
		return err
	}
	// Then delete the (now empty) bucket itself
	_, err := service.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: &bucket,
	})
	return err
}
