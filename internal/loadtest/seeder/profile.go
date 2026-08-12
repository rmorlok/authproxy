package seeder

import (
	"fmt"
	"os"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name        string            `yaml:"name" json:"name"`
	Namespace   string            `yaml:"namespace" json:"namespace"`
	Description string            `yaml:"description" json:"description"`
	Cluster     ProfileCluster    `yaml:"cluster" json:"cluster"`
	Objects     ProfileObjects    `yaml:"objects" json:"objects"`
	AuthProxy   ProfileAuthProxy  `yaml:"authproxy" json:"authproxy"`
	K6          ProfileK6         `yaml:"k6" json:"k6"`
	Acceptance  ProfileAcceptance `yaml:"acceptance" json:"acceptance"`
}

type ProfileCluster struct {
	Target string `yaml:"target" json:"target"`
}

type ProfileObjects struct {
	Namespaces             int         `yaml:"namespaces" json:"namespaces,omitempty"`
	NamespacesMin          int         `yaml:"namespacesMin" json:"namespacesMin,omitempty"`
	NamespacesMax          int         `yaml:"namespacesMax" json:"namespacesMax,omitempty"`
	Connections            int         `yaml:"connections" json:"connections"`
	StaleSetupConnections  int         `yaml:"staleSetupConnections" json:"staleSetupConnections,omitempty"`
	OAuthTokensExpiringPct PercentList `yaml:"oauthTokensExpiringPercent" json:"oauthTokensExpiringPercent,omitempty"`
	PeriodicProbePct       PercentList `yaml:"periodicProbePercent" json:"periodicProbePercent,omitempty"`
}

type ProfileAuthProxy struct {
	ApiReplicas      int `yaml:"apiReplicas" json:"apiReplicas"`
	WorkerReplicas   int `yaml:"workerReplicas" json:"workerReplicas"`
	PublicReplicas   int `yaml:"publicReplicas" json:"publicReplicas"`
	AdminApiReplicas int `yaml:"adminApiReplicas" json:"adminApiReplicas"`
}

type ProfileK6 struct {
	Mode                 string  `yaml:"mode" json:"mode"`
	Scenario             string  `yaml:"scenario" json:"scenario"`
	Rate                 int     `yaml:"rate" json:"rate"`
	Duration             string  `yaml:"duration" json:"duration"`
	TimeUnit             string  `yaml:"timeUnit" json:"timeUnit"`
	ConnectionRows       int     `yaml:"connectionRows" json:"connectionRows"`
	Parallelism          int     `yaml:"parallelism" json:"parallelism"`
	PreAllocatedVus      int     `yaml:"preAllocatedVus" json:"preAllocatedVus"`
	MaxVus               int     `yaml:"maxVus" json:"maxVus"`
	RequestTimeout       string  `yaml:"requestTimeout" json:"requestTimeout"`
	P95LatencyMs         int     `yaml:"p95LatencyMs" json:"p95LatencyMs"`
	MaxFailedRate        float64 `yaml:"maxFailedRate" json:"maxFailedRate"`
	UpstreamStatus       int     `yaml:"upstreamStatus" json:"upstreamStatus"`
	UpstreamBytes        int     `yaml:"upstreamBytes" json:"upstreamBytes"`
	UpstreamDelayMs      int     `yaml:"upstreamDelayMs" json:"upstreamDelayMs"`
	UpstreamJitterMs     int     `yaml:"upstreamJitterMs" json:"upstreamJitterMs"`
	UpstreamBearerPrefix string  `yaml:"upstreamBearerPrefix" json:"upstreamBearerPrefix"`
	UpstreamPathPrefix   string  `yaml:"upstreamPathPrefix" json:"upstreamPathPrefix"`
	SoakDuration         string  `yaml:"soakDuration" json:"soakDuration"`
	SpikeBaseRate        int     `yaml:"spikeBaseRate" json:"spikeBaseRate"`
	SpikeRate            int     `yaml:"spikeRate" json:"spikeRate"`
	SpikeRampUp          string  `yaml:"spikeRampUp" json:"spikeRampUp"`
	SpikeHold            string  `yaml:"spikeHold" json:"spikeHold"`
	SpikeRampDown        string  `yaml:"spikeRampDown" json:"spikeRampDown"`
	SpikeRecovery        string  `yaml:"spikeRecovery" json:"spikeRecovery"`
	ScaleReplicas        []int   `yaml:"scaleReplicas" json:"scaleReplicas"`
}

type ProfileAcceptance struct {
	Max5xxRate                          float64 `yaml:"max5xxRate" json:"max5xxRate"`
	P95LatencyTarget                    string  `yaml:"p95LatencyTarget" json:"p95LatencyTarget"`
	RefreshSweepMustDrainBeforeNextCron bool    `yaml:"refreshSweepMustDrainBeforeNextCron" json:"refreshSweepMustDrainBeforeNextCron"`
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
	if err := util.DecodeYAMLStrict(content, &profile); err != nil {
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
