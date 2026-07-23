package seeder

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name        string         `yaml:"name" json:"name"`
	Namespace   string         `yaml:"namespace" json:"namespace"`
	Description string         `yaml:"description" json:"description"`
	Objects     ProfileObjects `yaml:"objects" json:"objects"`
}

type ProfileObjects struct {
	Namespaces             int         `yaml:"namespaces" json:"namespaces,omitempty"`
	NamespacesMin          int         `yaml:"namespaces_min" json:"namespaces_min,omitempty"`
	NamespacesMax          int         `yaml:"namespaces_max" json:"namespaces_max,omitempty"`
	Connections            int         `yaml:"connections" json:"connections"`
	StaleSetupConnections  int         `yaml:"stale_setup_connections" json:"stale_setup_connections,omitempty"`
	OAuthTokensExpiringPct PercentList `yaml:"oauth_tokens_expiring_percent" json:"oauth_tokens_expiring_percent,omitempty"`
	PeriodicProbePct       PercentList `yaml:"periodic_probe_percent" json:"periodic_probe_percent,omitempty"`
}

type PercentList []int

func (p *PercentList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single int
		if err := value.Decode(&single); err != nil {
			return err
		}
		*p = []int{single}
		return nil
	case yaml.SequenceNode:
		var values []int
		if err := value.Decode(&values); err != nil {
			return err
		}
		*p = values
		return nil
	default:
		return fmt.Errorf("percent list must be a scalar or sequence")
	}
}

func LoadProfile(path string) (Profile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}

	var profile Profile
	if err := yaml.Unmarshal(content, &profile); err != nil {
		return Profile{}, err
	}
	if profile.Name == "" {
		return Profile{}, fmt.Errorf("profile name is required")
	}
	if profile.Objects.Connections < 0 {
		return Profile{}, fmt.Errorf("profile connections must be non-negative")
	}
	if profile.Objects.StaleSetupConnections < 0 {
		return Profile{}, fmt.Errorf("profile stale setup connections must be non-negative")
	}
	if profile.Namespace == "" {
		profile.Namespace = "authproxy-load"
	}
	return profile, nil
}

func (p Profile) TenantNamespaceCount() int {
	if p.Objects.Namespaces > 0 {
		return p.Objects.Namespaces
	}
	if p.Objects.NamespacesMin > 0 {
		return p.Objects.NamespacesMin
	}
	if p.Objects.Connections > 0 {
		return p.Objects.Connections
	}
	return 0
}

func (p Profile) DefaultOAuthExpiringPercent() int {
	return firstPercentOrZero(p.Objects.OAuthTokensExpiringPct)
}

func (p Profile) DefaultPeriodicProbePercent() int {
	return firstPercentOrZero(p.Objects.PeriodicProbePct)
}

func firstPercentOrZero(values []int) int {
	if len(values) == 0 {
		return 0
	}
	if values[0] < 0 {
		return 0
	}
	if values[0] > 100 {
		return 100
	}
	return values[0]
}
