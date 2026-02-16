package config

type Tasks struct {
	// Default retention for tasks unless a value is explicitly set
	DefaultRetention *HumanDuration `json:"default_retention,omitempty" yaml:"default_retention,omitempty"`
}
