package connectors

type Scope struct {
	Id       string `json:"id" yaml:"id"`
	Required *bool  `json:"required,omitempty" yaml:"required,omitempty"`
	Reason   string `json:"reason" yaml:"reason"`
}

func (s *Scope) IsRequired() bool {
	if s.Required == nil {
		// If unspecified, assume required
		return true
	}

	return *s.Required
}
