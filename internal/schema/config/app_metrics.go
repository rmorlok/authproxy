package config

import (
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type FullRequestRecording string

const (
	FullRequestRecordingNever  FullRequestRecording = "never"
	FullRequestRecordingAlways FullRequestRecording = "always"
)

// AppMetrics are the settings for application metrics storage and capture.
type AppMetrics struct {
	// Database is the database provider for app metrics. This can be the same database as the main database but would
	// typically be a data warehouse in production.
	Database *Database `json:"database" yaml:"database"`

	// BlobStorage configures the blob storage backend used for storing full request/response logs.
	// If not configured, full request-event logging will use an in-memory store (not suitable for production).
	BlobStorage *BlobStorage `json:"blobStorage,omitempty" yaml:"blobStorage,omitempty"`

	// ResourceSnapshotInterval is how often resource snapshot jobs should write metrics samples.
	// If unset, defaults to 15 minutes.
	ResourceSnapshotInterval *HumanDuration `json:"resourceSnapshotInterval,omitempty" yaml:"resourceSnapshotInterval,omitempty"`

	// RequestEvents configures request-event capture into the app metrics store.
	RequestEvents *AppMetricsRequestEvents `json:"requestEvents,omitempty" yaml:"requestEvents,omitempty"`
}

// AppMetricsRequestEvents are the settings related to capturing HTTP request events.
type AppMetricsRequestEvents struct {
	// Retention is how long the high-level logs should be retained. If unset, defaults to 30 days.
	Retention *HumanDuration `json:"retention" yaml:"retention"`

	// MaxRequestSize is the max size of request that will be stored. Values over this will be truncated.
	MaxRequestSize *HumanByteSize `json:"maxRequestSize,omitempty" yaml:"maxRequestSize,omitempty"`

	// MaxResponseSize is the max size of the response that will be stored. Values over this will be truncated.
	MaxResponseSize *HumanByteSize `json:"maxResponseSize,omitempty" yaml:"maxResponseSize,omitempty"`

	// MaxResponseWait is the maximum amount of time to wait for a response before logging it. Defaults to 60 seconds.
	MaxResponseWait *HumanDuration `json:"maxResponseWait" yaml:"maxResponseWait"`

	// FullRequestRecording flags if the full body/headers be logged for requests. Defaults to never, or can be enabled
	// with API calls to specific resources, or always on.
	FullRequestRecording *FullRequestRecording `json:"fullRequestRecording,omitempty" yaml:"fullRequestRecording,omitempty"`

	// FullRequestRetention is how long the full request events should be retained. If unset, defaults to 30 days.
	FullRequestRetention *HumanDuration `json:"fullRequestRetention,omitempty" yaml:"fullRequestRetention,omitempty"`

	// FlushInterval is how often buffered records are flushed the database. Defaults to 5s.
	FlushInterval *HumanDuration `json:"flushInterval,omitempty" yaml:"flushInterval,omitempty"`

	// FlushBatchSize is the number of records that triggers a flush. Defaults to 1000.
	FlushBatchSize *int `json:"flushBatchSize,omitempty" yaml:"flushBatchSize,omitempty"`
}

func (d *AppMetrics) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if d == nil {
		return vc.NewError("app_metrics block is required")
	}

	if d.Database == nil {
		result = multierror.Append(result, vc.PushField("database").NewError("database must be specified"))
	} else if err := d.Database.Validate(vc.PushField("database")); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func (d *AppMetrics) GetResourceSnapshotInterval() time.Duration {
	if d == nil || d.ResourceSnapshotInterval == nil {
		return 15 * time.Minute
	}

	return d.ResourceSnapshotInterval.Duration
}

func (d *AppMetrics) GetRequestEvents() *AppMetricsRequestEvents {
	if d == nil || d.RequestEvents == nil {
		return &AppMetricsRequestEvents{}
	}

	return d.RequestEvents
}

func (d *AppMetricsRequestEvents) GetRetention() time.Duration {
	if d == nil || d.Retention == nil {
		return 30 * 24 * time.Hour
	}

	return d.Retention.Duration
}

func (d *AppMetricsRequestEvents) GetFullRequestRecording() FullRequestRecording {
	if d == nil || d.FullRequestRecording == nil {
		return FullRequestRecordingNever
	}

	return *d.FullRequestRecording
}

func (d *AppMetricsRequestEvents) GetFullRequestRetention() time.Duration {
	if d.GetFullRequestRecording() == FullRequestRecordingNever {
		return 0
	}

	if d == nil || d.FullRequestRetention == nil {
		return 30 * 24 * time.Hour
	}

	return d.FullRequestRetention.Duration
}

func (d *AppMetricsRequestEvents) GetMaxRequestSize() uint64 {
	if d == nil || d.MaxRequestSize == nil {
		// Default value is 250kb
		return 250 * 1024
	}

	return d.MaxRequestSize.Value()
}

func (d *AppMetricsRequestEvents) GetMaxResponseSize() uint64 {
	if d == nil || d.MaxResponseSize == nil {
		// Default value is 250kb
		return 250 * 1024
	}

	return d.MaxResponseSize.Value()
}

func (d *AppMetricsRequestEvents) GetFlushInterval() time.Duration {
	if d == nil || d.FlushInterval == nil {
		return 5 * time.Second
	}

	return d.FlushInterval.Duration
}

func (d *AppMetricsRequestEvents) GetFlushBatchSize() int {
	if d == nil || d.FlushBatchSize == nil {
		return 1000
	}

	return *d.FlushBatchSize
}

func (d *AppMetricsRequestEvents) GetMaxResponseWait() time.Duration {
	if d == nil || d.MaxResponseWait == nil {
		return 60 * time.Second
	}

	return d.MaxResponseWait.Duration
}
