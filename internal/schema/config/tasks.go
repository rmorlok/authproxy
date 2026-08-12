package config

type Tasks struct {
	// Default retention for tasks unless a value is explicitly set
	DefaultRetention *HumanDuration `json:"defaultRetention,omitempty" yaml:"defaultRetention,omitempty"`
}
