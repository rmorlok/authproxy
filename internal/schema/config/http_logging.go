package config

import "time"

type FullRequestRecording string

const (
	FullRequestRecordingNever     FullRequestRecording = "never"
	FullRequestRecordingApiEnable FullRequestRecording = "api_enable"
	FullRequestRecordingAlways    FullRequestRecording = "always"
)

// HttpLogging are the settings related to logging HTTP requests.
type HttpLogging struct {
	// Enabled is whether any form of http logging is enabled. If true, this will log the high level details of requests
	// and responses, but not the body or headers unless additional configuration is present below.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// AutoMigrate controls if the migration to build the indexes for http logging happens automatically on startup.
	// If this value is not specified in the config, it defaults to true.
	AutoMigrate *bool `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`

	// Retention is how long the high-level logs should be retained. If unset, defaults to 30 days.
	Retention *HumanDuration `json:"retention" yaml:"retention"`

	// MaxRequestSize is the max size of request that will be stored. Values over this will be truncated.
	MaxRequestSize *HumanByteSize `json:"max_request_size,omitempty" yaml:"max_request_size,omitempty"`

	// MaxResponseSize is the max size of the response that will be stored. Values over this will be truncated.
	MaxResponseSize *HumanByteSize `json:"max_response_size,omitempty" yaml:"max_response_size,omitempty"`

	// MaxResponseWait is the maximum amount of time to wait for a response before logging it. Defaults to 60 seconds.
	MaxResponseWait *HumanDuration `json:"max_response_wait" yaml:"max_response_wait"`

	// FullRequestRecording flags if the full body/headers be logged for requests. Defaults to never, or can be enabled
	// with API calls to specific resources, or always on.
	FullRequestRecording *FullRequestRecording `json:"full_request_recording,omitempty" yaml:"full_request_recording,omitempty"`

	// FullRequestRetention is how long the full request logs should be retained. If unset, defaults to 30 days.
	FullRequestRetention *HumanDuration `json:"full_request_retention,omitempty" yaml:"full_request_retention,omitempty"`

	// BlobStorage configures the blob storage backend used for storing full request/response logs.
	// If not configured, full request logging will use an in-memory store (not suitable for production).
	BlobStorage *BlobStorage `json:"blob_storage,omitempty" yaml:"blob_storage,omitempty"`
}

func (d *HttpLogging) IsEnabled() bool {
	if d == nil {
		return false
	}

	return d.Enabled
}

func (d *HttpLogging) GetAutoMigrate() bool {
	if !d.IsEnabled() {
		return false
	}

	if d.AutoMigrate == nil {
		return true
	}

	return *d.AutoMigrate
}

func (d *HttpLogging) GetRetention() time.Duration {
	if !d.IsEnabled() {
		return 0
	}

	if d.Retention == nil {
		return 30 * 24 * time.Hour
	}

	return d.Retention.Duration
}

func (d *HttpLogging) GetFullRequestRecording() FullRequestRecording {
	if !d.IsEnabled() {
		return FullRequestRecordingNever
	}

	if d.FullRequestRecording == nil {
		return FullRequestRecordingNever
	}

	return *d.FullRequestRecording
}

func (d *HttpLogging) GetFullRequestRetention() time.Duration {
	if d.GetFullRequestRecording() == FullRequestRecordingNever {
		return 0
	}

	if d.FullRequestRetention == nil {
		return 30 * 24 * time.Hour
	}

	return d.FullRequestRetention.Duration
}

func (d *HttpLogging) GetMaxRequestSize() uint64 {
	if !d.IsEnabled() {
		return 0
	}

	if d.MaxRequestSize == nil {
		// Default value is 250kb
		return 250 * 1024
	}

	return d.MaxRequestSize.Value()
}

func (d *HttpLogging) GetMaxResponseSize() uint64 {
	if !d.IsEnabled() {
		return 0
	}

	if d.MaxResponseSize == nil {
		// Default value is 250kb
		return 250 * 1024
	}

	return d.MaxResponseSize.Value()
}

func (d *HttpLogging) GetMaxResponseWait() time.Duration {
	if d.MaxResponseWait == nil {
		return 60 * time.Second
	}

	return d.MaxResponseWait.Duration
}
