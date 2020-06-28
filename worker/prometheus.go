package main

import (
	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/mulbc/gosbench/common"
	prom "github.com/prometheus/client_golang/prometheus"
	promModel "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
)

var pe *prometheus.Exporter
var promRegistry = prom.NewRegistry()
var promTestStart = prom.NewGaugeVec(
	prom.GaugeOpts{
		Name:      "test_start",
		Namespace: "gosbench",
		Help:      "Determines the start time of a job for Grafana annotations",
	}, []string{"testName"})
var promTestEnd = prom.NewGaugeVec(
	prom.GaugeOpts{
		Name:      "test_end",
		Namespace: "gosbench",
		Help:      "Determines the end time of a job for Grafana annotations",
	}, []string{"testName"})
var promFinishedOps = prom.NewCounterVec(
	prom.CounterOpts{
		Name:      "finished_ops",
		Namespace: "gosbench",
		Help:      "Finished S3 operations",
	}, []string{"testName", "method"})
var promFailedOps = prom.NewCounterVec(
	prom.CounterOpts{
		Name:      "failed_ops",
		Namespace: "gosbench",
		Help:      "Failed S3 operations",
	}, []string{"testName", "method"})
var promLatency = prom.NewHistogramVec(
	prom.HistogramOpts{
		Name:      "ops_latency",
		Namespace: "gosbench",
		Help:      "Histogram latency of S3 operations",
		Buckets:   prom.ExponentialBuckets(2, 2, 12),
	}, []string{"testName", "method"})
var promUploadedBytes = prom.NewCounterVec(
	prom.CounterOpts{
		Name:      "uploaded_bytes",
		Namespace: "gosbench",
		Help:      "Uploaded bytes to S3 store",
	}, []string{"testName", "method"})
var promDownloadedBytes = prom.NewCounterVec(
	prom.CounterOpts{
		Name:      "downloaded_bytes",
		Namespace: "gosbench",
		Help:      "Downloaded bytes from S3 store",
	}, []string{"testName", "method"})

func init() {
	// Then create the prometheus stat exporter
	var err error
	pe, err = prometheus.NewExporter(prometheus.Options{
		Namespace: "gosbench",
		ConstLabels: map[string]string{
			"version": "0.0.1",
		},
		Registry: promRegistry,
	})
	if err != nil {
		log.WithError(err).Fatalf("Failed to create the Prometheus exporter:")
	}

	if err = promRegistry.Register(promTestStart); err != nil {
		log.WithError(err).Error("Issues when adding test_start gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promTestEnd); err != nil {
		log.WithError(err).Error("Issues when adding test_end gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promFinishedOps); err != nil {
		log.WithError(err).Error("Issues when adding finished_ops gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promFailedOps); err != nil {
		log.WithError(err).Error("Issues when adding failed_ops gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promLatency); err != nil {
		log.WithError(err).Error("Issues when adding ops_latency gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promUploadedBytes); err != nil {
		log.WithError(err).Error("Issues when adding uploaded_bytes gauge to Prometheus registry")
	}
	if err = promRegistry.Register(promDownloadedBytes); err != nil {
		log.WithError(err).Error("Issues when adding downloaded_bytes gauge to Prometheus registry")
	}
}

func getCurrentPromValues(testName string) common.BenchmarkResult {
	benchResult := common.BenchmarkResult{
		TestName: testName,
	}
	result, err := promRegistry.Gather()
	if err != nil {
		log.WithError(err).Error("ERROR during PROM VALUE gathering")
	}
	resultmap := map[string][]*promModel.Metric{}
	for _, metric := range result {
		resultmap[*metric.Name] = metric.Metric
	}
	benchResult.Operations = sumCounterForTest(resultmap["gosbench_finished_ops"], testName)
	benchResult.Bytes = sumCounterForTest(resultmap["gosbench_uploaded_bytes"], testName) + sumCounterForTest(resultmap["gosbench_downloaded_bytes"], testName)
	benchResult.LatencyAvg = averageHistogramForTest(resultmap["gosbench_ops_latency"], testName)
	return benchResult
}

func sumCounterForTest(metrics []*promModel.Metric, testName string) float64 {
	sum := float64(0)
	for _, metric := range metrics {
		for _, label := range metric.Label {
			if *label.Name == "testName" && *label.Value == testName {
				sum += *metric.Counter.Value
			}
		}
	}
	return sum
}

func averageHistogramForTest(metrics []*promModel.Metric, testName string) float64 {
	sum := float64(0)
	count := float64(0)
	for _, metric := range metrics {
		for _, label := range metric.Label {
			if *label.Name == "testName" && *label.Value == testName {
				sum += *metric.Histogram.SampleSum
				count += float64(*metric.Histogram.SampleCount)
			}
		}
	}
	return sum / count
}
