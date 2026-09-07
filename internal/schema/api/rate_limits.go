package api

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

const RateLimitDryRunActionKind meta.Kind = "RateLimitDryRun"

type ListRateLimitsResponseJson struct {
	apiv1alpha1.ResourceList[rlschema.RateLimit] `json:",inline" yaml:",inline"`
}

func NewListRateLimitsResponseJson(
	items []rlschema.RateLimit,
	continueToken string,
) ListRateLimitsResponseJson {
	return ListRateLimitsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			rlschema.RateLimitKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

// ProxyRequestJson is the wire shape used by API endpoints that accept a
// synthetic proxy request, such as rate-limit dry-run.
//
//	@Description	Request to proxy or simulate an HTTP request
type ProxyRequestJson struct {
	URL      string                       `json:"url" yaml:"url" example:"https://api.example.com/v1/users"`
	Method   string                       `json:"method" yaml:"method" example:"GET"`
	Headers  map[string]common.HeadersVal `json:"headers,omitempty" yaml:"headers,omitempty" swaggertype:"object"`
	Labels   map[string]string            `json:"labels,omitempty" yaml:"labels,omitempty"`
	BodyRaw  []byte                       `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
	BodyJson interface{}                  `json:"bodyJson,omitempty" yaml:"bodyJson,omitempty"`
}

// RateLimitDryRunSpec describes synthetic traffic evaluated without consuming
// a rate-limit counter. metadata.target identifies either a Connection or a
// Namespace; ActorRef optionally supplies actor context.
type RateLimitDryRunSpec struct {
	Request     ProxyRequestJson      `json:"request" yaml:"request"`
	RequestType string                `json:"requestType" yaml:"requestType" example:"proxy"`
	ActorRef    *meta.ObjectReference `json:"actorRef,omitempty" yaml:"actorRef,omitempty"`
}

type RateLimitDryRunStatus struct {
	RequestLabelSnapshot map[string]string      `json:"requestLabelSnapshot" yaml:"requestLabelSnapshot"`
	Matched              []DryRunMatchJson      `json:"matched" yaml:"matched"`
	NotMatched           []DryRunNotMatchedJson `json:"notMatched" yaml:"notMatched"`
}

type RateLimitDryRunAction struct {
	apiv1alpha1.Action[RateLimitDryRunSpec, RateLimitDryRunStatus] `json:",inline" yaml:",inline"`
}

func (a *RateLimitDryRunAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return a.validateFields(false)
}

func (a *RateLimitDryRunAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	return a.validateFields(true)
}

func (a *RateLimitDryRunAction) validateFields(requireStatus bool) error {
	if err := validateDryRunTarget(a.Metadata.Target); err != nil {
		return err
	}
	if a.Spec.ActorRef != nil {
		vc := &common.ValidationContext{Path: "$.spec.actorRef"}
		if err := meta.ValidateObjectReferenceWithOptions(
			*a.Spec.ActorRef,
			meta.ObjectReferenceValidationOptions{
				ExpectedAPIVersion: meta.APIVersionV1Alpha1,
				ExpectedKind:       actorschema.ActorKind,
				IDValidator:        actorschema.ValidateID,
			},
			vc,
		); err != nil {
			return err
		}
		if a.Spec.ActorRef.ID == "" || a.Spec.ActorRef.Name != "" ||
			a.Spec.ActorRef.Namespace != "" || a.Spec.ActorRef.Generation != 0 {
			return vc.NewError("actor references support id only")
		}
	}
	if a.Spec.Request.Method == "" {
		return fmt.Errorf("$.spec.request.method: is required")
	}
	if a.Spec.Request.URL == "" {
		return fmt.Errorf("$.spec.request.url: is required")
	}
	if a.Spec.RequestType == "" {
		return fmt.Errorf("$.spec.requestType: is required")
	}
	if requireStatus && a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	return nil
}

func validateDryRunTarget(target meta.ObjectReference) error {
	vc := &common.ValidationContext{Path: "$.metadata.target"}
	options := meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       target.Kind,
	}
	switch target.Kind {
	case connectionschema.ConnectionKind:
		options.IDValidator = connectionschema.ValidateID
	case namespaceschema.NamespaceKind:
		options.IDValidator = namespaceschema.ValidatePath
	default:
		return vc.NewErrorForField("kind", "must be Connection or Namespace")
	}
	if err := meta.ValidateObjectReferenceWithOptions(target, options, vc); err != nil {
		return err
	}
	if target.ID == "" || target.Name != "" || target.Namespace != "" || target.Generation != 0 {
		return vc.NewError("rate-limit dry-run targets support id only")
	}
	return nil
}

func NewRateLimitDryRunResponse(
	target meta.ObjectReference,
	spec RateLimitDryRunSpec,
	status RateLimitDryRunStatus,
) RateLimitDryRunAction {
	return RateLimitDryRunAction{Action: apiv1alpha1.NewActionResponse(
		RateLimitDryRunActionKind,
		target,
		spec,
		status,
	)}
}

type DryRunMatchJson struct {
	RateLimitId      apid.ID `json:"rateLimitId" yaml:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	EffectiveMode    string  `json:"effectiveMode" yaml:"effectiveMode" example:"enforce"`
	BucketKey        string  `json:"bucketKey" yaml:"bucketKey" example:"rate_limit:rl_test550e8400abcde:actor:act_test"`
	AlgorithmSummary string  `json:"algorithmSummary" yaml:"algorithmSummary" example:"fixed_window window=1m limit=100"`
	WouldAllow       bool    `json:"wouldAllow" yaml:"wouldAllow" example:"true"`
	Remaining        int     `json:"remaining" yaml:"remaining" example:"99"`
	RetryAfterMs     int64   `json:"retryAfterMs" yaml:"retryAfterMs" example:"0"`
	PeekFailed       bool    `json:"peekFailed" yaml:"peekFailed" example:"false"`
}

type DryRunNotMatchedJson struct {
	RateLimitId apid.ID `json:"rateLimitId" yaml:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string  `json:"namespace" yaml:"namespace" example:"root.acme"`
	Reason      string  `json:"reason" yaml:"reason" example:"method did not match"`
}
