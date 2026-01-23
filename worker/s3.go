package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	log "github.com/sirupsen/logrus"

	"github.com/mulbc/gosbench/common"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
)

var svc, housekeepingSvc *s3.Client
var ctx context.Context
var hc *http.Client

// redirectTransport handles 307/308 redirects with body replay.
// Uses a probe request (empty body) to discover redirect location first,
// then sends the full body directly to the target. This avoids "connection reset by peer"
// when AIStore closes the connection during body transfer to issue a redirect.
type redirectTransport struct {
	base         http.RoundTripper
	maxRedirects int
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Buffer body once so we can replay it on redirects
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
	}

	// For PUT/POST with body, send probe first to discover redirect location
	// This avoids connection reset when server redirects during body transfer
	if len(bodyBytes) > 0 && (req.Method == http.MethodPut || req.Method == http.MethodPost) {
		probeReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create probe request: %w", err)
		}

		// Copy headers but set Content-Length to 0 for probe
		for key, vals := range req.Header {
			for _, val := range vals {
				probeReq.Header.Add(key, val)
			}
		}
		probeReq.ContentLength = 0
		probeReq.Header.Set("Content-Length", "0")

		probeResp, err := t.base.RoundTrip(probeReq)
		if err != nil {
			log.Debugf("Probe request failed, falling back to direct request: %v", err)
		} else {
			defer probeResp.Body.Close()

			// If we got a redirect, send body directly to target
			if probeResp.StatusCode == http.StatusTemporaryRedirect || probeResp.StatusCode == http.StatusPermanentRedirect {
				location := probeResp.Header.Get("Location")
				if location != "" {
					log.Debugf("Redirect (probe): %s %s -> %s", req.Method, req.URL.String(), location)

					newURL, err := req.URL.Parse(location)
					if err != nil {
						return nil, fmt.Errorf("failed to parse redirect location: %w", err)
					}

					// Send full request directly to target
					redirectReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL.String(), bytes.NewReader(bodyBytes))
					if err != nil {
						return nil, fmt.Errorf("failed to create redirect request: %w", err)
					}

					for key, vals := range req.Header {
						for _, val := range vals {
							redirectReq.Header.Add(key, val)
						}
					}
					redirectReq.ContentLength = int64(len(bodyBytes))
					redirectReq.GetBody = func() (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(bodyBytes)), nil
					}

					return t.base.RoundTrip(redirectReq)
				}
			}
		}
	}

	// Set up body and GetBody for the request
	if bodyBytes != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	// Fallback redirect loop for GET/DELETE or when probe doesn't get a redirect
	for redirects := 0; redirects <= t.maxRedirects; redirects++ {
		// Reset body reader for each attempt
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		// Not a redirect - return response
		if resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusPermanentRedirect {
			return resp, nil
		}

		location := resp.Header.Get("Location")
		if location == "" {
			return resp, nil
		}

		log.Debugf("Redirect %d: %s %s -> %s", redirects+1, req.Method, req.URL.String(), location)
		resp.Body.Close()

		// Parse redirect location
		newURL, err := req.URL.Parse(location)
		if err != nil {
			return nil, fmt.Errorf("failed to parse redirect location: %w", err)
		}

		// Create new request for redirect target
		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create redirect request: %w", err)
		}

		// Copy headers
		for key, vals := range req.Header {
			for _, val := range vals {
				newReq.Header.Add(key, val)
			}
		}

		// Set up body for redirect request
		if bodyBytes != nil {
			newReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			newReq.ContentLength = int64(len(bodyBytes))
			newReq.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		req = newReq
	}

	return nil, fmt.Errorf("too many redirects (max %d)", t.maxRedirects)
}


func init() {
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
		log.Infof("Starting Prometheus Exporter on port %d", prometheusPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", prometheusPort), mux); err != nil {
			log.WithError(err).Fatalf("Failed to run Prometheus /metrics endpoint:")
		}
	}()

}

// InitS3 initialises the S3 session
// Also starts the Prometheus exporter on Port 8888
func InitS3(config common.S3Configuration) {
	// All clients require a Session. The Session provides the client with
	// shared configuration such as region, endpoint, and credentials. A
	// Session should be shared where possible to take advantage of
	// configuration and credential caching. See the session package for
	// more information.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipSSLVerify},
	}
	tr2 := &ochttp.Transport{Base: tr}
	
	// Conditionally wrap with redirectTransport to handle 307/308 redirects with body
	// Enable this for AIStore or other S3-compatible storage that uses redirects
	var transport http.RoundTripper = tr2
	if config.HandleRedirect {
		log.Info("Redirect handling enabled")
		transport = &redirectTransport{
			base:         tr2,
			maxRedirects: 5,
		}
	}
	hc = &http.Client{
		Transport: transport,
	}


	// TODO Create a context with a timeout - we already use this context in all S3 calls
	// Usually this shouldn't be a problem ;)
	ctx = context.Background()

	cfg, err := s3config.LoadDefaultConfig(ctx,
		s3config.WithHTTPClient(hc),
		s3config.WithRegion(config.Region),
		s3config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKey, config.SecretKey, "")),
		s3config.WithRetryer(func() aws.Retryer {
			return aws.NopRetryer{}
		}),
	)
	if err != nil {
		log.WithError(err).Fatal("Unable to build S3 config")
	}
	// Use this Session to do things that are hidden from the performance monitoring
	// Setting up the housekeeping S3 client
	var hkTransport http.RoundTripper = tr
	if config.HandleRedirect {
		hkTransport = &redirectTransport{
			base:         tr,
			maxRedirects: 5,
		}
	}
	hkhc := &http.Client{
		Transport: hkTransport,
	}

	hkCfg, err := s3config.LoadDefaultConfig(ctx,
		s3config.WithHTTPClient(hkhc),
		s3config.WithRegion(config.Region),
		s3config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKey, config.SecretKey, "")),
		s3config.WithRetryer(func() aws.Retryer {
			return aws.NopRetryer{}
		}),
	)
	if err != nil {
		log.WithError(err).Fatal("Unable to build S3 housekeeping config")
	}

	// Create a new instance of the service's client with a Session.
	// Optional aws.Config values can also be provided as variadic arguments
	// to the New function. This option allows you to provide service
	// specific configuration.
	svc = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(config.Endpoint)
		o.UsePathStyle = config.UsePathStyle
	})
	// Use this service to do things that are hidden from the performance monitoring
	housekeepingSvc = s3.NewFromConfig(hkCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(config.Endpoint)
		o.UsePathStyle = config.UsePathStyle
	})

	log.Debug("S3 Init done")
}

func putObject(service *s3.Client, objectName string, objectContent io.ReadSeeker, bucket string) error {
	// Create an uploader with S3 client and custom options
	uploader := manager.NewUploader(service, func(d *manager.Uploader) {
		d.MaxUploadParts = 1
	})

	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
		Body:   objectContent,
	})

	if err != nil {
		log.WithError(err).WithField("object", objectName).WithField("bucket", bucket).Errorf("Failed to upload object,")
		return err
	}

	log.WithField("bucket", bucket).WithField("key", objectName).Tracef("Upload successful")

	return err
}

// func getObjectProperties(service *s3.S3, objectName string, bucket string) {
// 	service.ListObjects(&s3.ListObjectsInput{
// 		Bucket: &bucket,
// 	})
// 	result, err := service.GetObjectWithContext(ctx, &s3.GetObjectInput{
// 		Bucket: &bucket,
// 		Key:    &objectName,
// 	})
// 	if err != nil {
// 		// Cast err to awserr.Error to handle specific error codes.
// 		aerr, ok := err.(awserr.Error)
// 		if ok && aerr.Code() == s3.ErrCodeNoSuchKey {
// 			log.WithError(aerr).Errorf("Could not find object %s in bucket %s when querying properties", objectName, bucket)
// 		}
// 	}

// 	// Make sure to close the body when done with it for S3 GetObject APIs or
// 	// will leak connections.
// 	defer result.Body.Close()

// 	log.Debugf("Object Properties:\n%+v", result)
// }

func listObjects(service *s3.Client, prefix string, bucket string) ([]types.Object, error) {
	var bucketContents []types.Object
	p := s3.NewListObjectsV2Paginator(service, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		// Next Page takes a new context for each page retrieval. This is where
		// you could add timeouts or deadlines.
		page, err := p.NextPage(ctx)
		if err != nil {
			log.WithError(err).WithField("prefix", prefix).WithField("bucket", bucket).Errorf("Failed to list objects")
			return nil, err
		}
		bucketContents = append(bucketContents, page.Contents...)
	}

	return bucketContents, nil
}

func getObject(service *s3.Client, objectName string, bucket string, objectSize uint64) error {
	// Remove the allocation of buffer
	result, err := service.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
	})
	if err != nil {
		return err
	}
	numBytes, err := io.Copy(io.Discard, result.Body)
	if err != nil {
		return err
	}
	if numBytes != int64(objectSize) {
		return fmt.Errorf("Expected object length %d is not matched to actual object length %d", objectSize, numBytes)
	}
	return nil
}

func deleteObject(service *s3.Client, objectName string, bucket string) error {
	_, err := service.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &objectName,
	})
	if err != nil {
		log.WithError(err).Errorf("Could not find object %s in bucket %s for deletion", objectName, bucket)
	}
	return err
}

func createBucket(service *s3.Client, bucket string) error {
	// Do not err when the bucket is already there...
	_, err := service.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		var bne *types.BucketAlreadyExists
		// Ignore error if bucket already exists
		if errors.As(err, &bne) {
			return nil
		}
		log.WithError(err).Errorf("Issues when creating bucket %s", bucket)
	}
	return err
}

func deleteBucket(service *s3.Client, bucket string) error {
	// First delete all objects in the bucket
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	var bucketContents []types.Object
	isTruncated := true
	for isTruncated {
		result, err := service.ListObjectsV2(ctx, input)
		if err != nil {
			return err
		}
		bucketContents = append(bucketContents, result.Contents...)
		input.ContinuationToken = result.NextContinuationToken
		isTruncated = *result.IsTruncated
	}

	if len(bucketContents) > 0 {
		var objectsToDelete []types.ObjectIdentifier
		for _, item := range bucketContents {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key: item.Key,
			})
		}

		deleteObjectsInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{
				Objects: objectsToDelete,
				Quiet:   aws.Bool(true),
			},
		}

		_, err := svc.DeleteObjects(ctx, deleteObjectsInput)
		if err != nil {
			return err
		}
	}

	// Then delete the (now empty) bucket itself
	_, err := service.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: &bucket,
	})
	return err
}
