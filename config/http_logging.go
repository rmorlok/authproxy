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

	// Retention is how long the high-level logs should be retained. If unset, defaults to 30 days.
	Retention *HumanDuration `json:"retention" yaml:"retention"`

	// FullRequestRecording flags if the full body/headers be logged for requests. Defaults to never, or can be enabled
	// with API calls to specific resources, or always on.
	FullRequestRecording *FullRequestRecording `json:"full_request_recording,omitempty" yaml:"full_request_recording,omitempty"`

	// FullRequestRetention is how long the full request logs should be retained. If unset, defaults to 30 days.
	FullRequestRetention *HumanDuration `json:"full_request_retention,omitempty" yaml:"full_request_retention,omitempty"`
}

func (d *HttpLogging) IsEnabled() bool {
	if d == nil {
		return false
	}

	return d.Enabled
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
