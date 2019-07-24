package main

import (
	"context"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/mulbc/gosbench/common"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
)

var svc, housekeepingSvc *s3.S3
var ctx context.Context

// InitS3 initialises the S3 session
// Also starts the Prometheus exporter on Port 8888
func InitS3(config common.S3Configuration) {
	// Then create the prometheus stat exporter
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "gosbench",
		ConstLabels: map[string]string{
			"version": "0.0.1",
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
		// TODO Also set the remaining S3 connection details...
		Region:           &config.Region,
		Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
		Endpoint:         &config.Endpoint,
		S3ForcePathStyle: aws.Bool(true),
	}))
	// Use this Session to do things that are hidden from the performance monitoring
	housekeepingSess := session.Must(session.NewSession(&aws.Config{
		// TODO Also set the remaining S3 connection details...
		Region:           &config.Region,
		Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
		Endpoint:         &config.Endpoint,
		S3ForcePathStyle: aws.Bool(true),
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
	// Use this service to do things that are hidden from the performance monitoring
	housekeepingSvc = s3.New(housekeepingSess)

	// TODO Create a context with a timeout - we already use this context in all S3 calls
	// Usually this shouldn't be a problem ;)
	ctx = context.Background()
	log.Debug("S3 Init done")
}

func putObject(service *s3.S3, objectName string, objectContent io.ReadSeeker, bucket string) error {
	// Create an uploader with S3 client and custom options
	uploader := s3manager.NewUploaderWithClient(service)

	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &bucket,
		Key:    &objectName,
		Body:   objectContent,
	}, func(d *s3manager.Uploader) {
		d.MaxUploadParts = 1
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			// If the SDK can determine the request or retry delay was canceled
			// by a context the CanceledErrorCode error code will be returned.
			log.WithError(aerr).Errorf("Upload canceled due to timeout")
		} else {
			log.WithError(err).WithField("object", objectName).WithField("bucket", bucket).Errorf("Failed to upload object,")
		}
		return err
	}

	log.WithField("bucket", bucket).WithField("key", objectName).Tracef("Upload successful")
	return err
}

func getObjectProperties(service *s3.S3, objectName string, bucket string) {
	service.ListObjects(&s3.ListObjectsInput{
		Bucket: &bucket,
	})
	result, err := service.GetObjectWithContext(ctx, &s3.GetObjectInput{
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
func listObjects(service *s3.S3, prefix string, bucket string) error {
	_, err := service.ListObjects(&s3.ListObjectsInput{
		Bucket: &bucket,
		Prefix: &prefix,
	})
	if err != nil {
		// Cast err to awserr.Error to handle specific error codes.
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == s3.ErrCodeNoSuchKey {
			log.WithError(aerr).Errorf("Could not find prefix %s in bucket %s when querying properties", prefix, bucket)
		}
	}
	return err
}

func getObject(service *s3.S3, objectName string, bucket string) error {
	// Create a downloader with the session and custom options
	downloader := s3manager.NewDownloaderWithClient(service)
	buf := aws.NewWriteAtBuffer([]byte{})
	_, err := downloader.DownloadWithContext(ctx, buf, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
	}, func(d *s3manager.Downloader) {
		d.PartSize = 64 * 1024 * 1024 // 64MB parts
	})
	return err
}

func deleteObject(service *s3.S3, objectName string, bucket string) error {
	_, err := service.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: &bucket,
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
	}
	return err
}

func createBucket(service *s3.S3, bucket string) error {
	// TODO do not err when the bucket is already there...
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
