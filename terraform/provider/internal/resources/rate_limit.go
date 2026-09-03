package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ resource.Resource = &RateLimitResource{}
var _ resource.ResourceWithImportState = &RateLimitResource{}
var _ resource.ResourceWithConfigValidators = &RateLimitResource{}

type RateLimitResource struct {
	client *client.Client
}

// RateLimitResourceModel mirrors the HCL the user writes. Top-level
// attributes plus three nested blocks (selector, bucket, algorithm) so
// authors get autocomplete and field-level plan diffs — no jsonencode
// blobs.
type RateLimitResourceModel struct {
	Id          types.String             `tfsdk:"id"`
	Namespace   types.String             `tfsdk:"namespace"`
	Mode        types.String             `tfsdk:"mode"`
	Labels      types.Map                `tfsdk:"labels"`
	Annotations types.Map                `tfsdk:"annotations"`
	Scope       *rateLimitScopeModel     `tfsdk:"scope"`
	Selector    *rateLimitSelectorModel  `tfsdk:"selector"`
	Bucket      *rateLimitBucketModel    `tfsdk:"bucket"`
	Algorithm   *rateLimitAlgorithmModel `tfsdk:"algorithm"`
	CreatedAt   types.String             `tfsdk:"created_at"`
	UpdatedAt   types.String             `tfsdk:"updated_at"`
}

type rateLimitScopeModel struct {
	NamespaceMatcher types.String                 `tfsdk:"namespace_matcher"`
	ConnectorRef     *rateLimitConnectorRefModel  `tfsdk:"connector_ref"`
	ConnectionRef    *rateLimitConnectionRefModel `tfsdk:"connection_ref"`
}

type rateLimitConnectorRefModel struct {
	ID         types.String `tfsdk:"id"`
	Generation types.Int64  `tfsdk:"generation"`
}

type rateLimitConnectionRefModel struct {
	ID types.String `tfsdk:"id"`
}

type rateLimitSelectorModel struct {
	LabelSelector types.String             `tfsdk:"label_selector"`
	Methods       types.List               `tfsdk:"methods"`
	RequestTypes  types.List               `tfsdk:"request_types"`
	PathMatch     *rateLimitPathMatchModel `tfsdk:"path_match"`
}

type rateLimitPathMatchModel struct {
	Kind  types.String `tfsdk:"kind"`
	Value types.String `tfsdk:"value"`
}

type rateLimitBucketModel struct {
	Dimensions types.List `tfsdk:"dimensions"`
}

type rateLimitAlgorithmModel struct {
	FixedWindow   *rateLimitFixedWindowModel   `tfsdk:"fixed_window"`
	SlidingWindow *rateLimitSlidingWindowModel `tfsdk:"sliding_window"`
	TokenBucket   *rateLimitTokenBucketModel   `tfsdk:"token_bucket"`
}

type rateLimitFixedWindowModel struct {
	Window types.String `tfsdk:"window"`
	Limit  types.Int64  `tfsdk:"limit"`
}

type rateLimitSlidingWindowModel struct {
	Window types.String `tfsdk:"window"`
	Limit  types.Int64  `tfsdk:"limit"`
	Mode   types.String `tfsdk:"mode"`
}

type rateLimitTokenBucketModel struct {
	Capacity   types.Int64   `tfsdk:"capacity"`
	RefillRate types.Float64 `tfsdk:"refill_rate"`
}

func NewRateLimitResource() resource.Resource {
	return &RateLimitResource{}
}

func (r *RateLimitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rate_limit"
}

func (r *RateLimitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy rate-limit resource. Every field maps to a typed HCL attribute so authors get plan-time validation and field-level diffs — no jsonencode required.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The rate-limit ID (server-assigned).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The namespace this rate limit belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Description: "Either 'enforce' (default) or 'observe'. In observe mode the rule evaluates and records matches but never returns a 429.",
				Optional:    true,
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "User labels on the rate limit.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations on the rate limit.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
		Blocks: map[string]schema.Block{
			"scope": schema.SingleNestedBlock{
				Description: "Optionally narrow the owning namespace cascade to one namespace matcher, connector, or connection.",
				Attributes: map[string]schema.Attribute{
					"namespace_matcher": schema.StringAttribute{
						Description: "A namespace matcher at or below the rate limit namespace, such as root.acme.payments.**.",
						Optional:    true,
					},
				},
				Blocks: map[string]schema.Block{
					"connector_ref": schema.SingleNestedBlock{
						Description: "Target one connector. Omit generation to include every connector generation.",
						Attributes: map[string]schema.Attribute{
							"id":         schema.StringAttribute{Required: true},
							"generation": schema.Int64Attribute{Optional: true},
						},
					},
					"connection_ref": schema.SingleNestedBlock{
						Description: "Target one connection.",
						Attributes: map[string]schema.Attribute{
							"id": schema.StringAttribute{Required: true},
						},
					},
				},
			},
			"selector": schema.SingleNestedBlock{
				Description: "Match criteria. All non-empty clauses are ANDed.",
				Attributes: map[string]schema.Attribute{
					"label_selector": schema.StringAttribute{
						Description: "Kubernetes-style selector evaluated against the per-request label snapshot.",
						Optional:    true,
					},
					"methods": schema.ListAttribute{
						Description: "HTTP verbs the rule applies to. Empty / omitted = any.",
						Optional:    true,
						ElementType: types.StringType,
					},
					"request_types": schema.ListAttribute{
						Description: "Request types the rule applies to. Omit to use the default [proxy, probe]; an empty list is rejected by the server.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
				Blocks: map[string]schema.Block{
					"path_match": schema.SingleNestedBlock{
						Description: "Match the request's final upstream URL path.",
						Attributes: map[string]schema.Attribute{
							"kind": schema.StringAttribute{
								Description: "One of 'prefix', 'glob', or 'regex'.",
								Optional:    true,
							},
							"value": schema.StringAttribute{
								Description: "The path expression interpreted per 'kind'.",
								Optional:    true,
							},
						},
					},
				},
			},
			"bucket": schema.SingleNestedBlock{
				Description: "Projects matched requests into independent counters.",
				Attributes: map[string]schema.Attribute{
					"dimensions": schema.ListAttribute{
						Description: "Ordered list of dimension references. Reserved names: actor, connection, connector, connector_version, namespace, method. Label values via labels/<key>. Empty list = single global bucket per rule.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
			"algorithm": schema.SingleNestedBlock{
				Description: "Tagged union: exactly one of fixed_window, sliding_window, or token_bucket must be set.",
				Blocks: map[string]schema.Block{
					"fixed_window": schema.SingleNestedBlock{
						Description: "Fixed-window counter. Resets at floor(now/window) boundaries.",
						Attributes: map[string]schema.Attribute{
							"window": schema.StringAttribute{
								Description: "Window length as a HumanDuration (e.g. '1m', '5m').",
								Optional:    true,
							},
							"limit": schema.Int64Attribute{
								Description: "Maximum requests per window.",
								Optional:    true,
							},
						},
					},
					"sliding_window": schema.SingleNestedBlock{
						Description: "Sliding-window counter. Mode 'log' = exact (ZSET); 'counter' = approximate (two adjacent counters).",
						Attributes: map[string]schema.Attribute{
							"window": schema.StringAttribute{
								Description: "Window length as a HumanDuration.",
								Optional:    true,
							},
							"limit": schema.Int64Attribute{
								Description: "Maximum requests within the trailing window.",
								Optional:    true,
							},
							"mode": schema.StringAttribute{
								Description: "'log' (exact) or 'counter' (approximate).",
								Optional:    true,
							},
						},
					},
					"token_bucket": schema.SingleNestedBlock{
						Description: "Token-bucket rate limit with refill rate.",
						Attributes: map[string]schema.Attribute{
							"capacity": schema.Int64Attribute{
								Description: "Maximum tokens the bucket can hold (burst capacity).",
								Optional:    true,
							},
							"refill_rate": schema.Float64Attribute{
								Description: "Tokens added per second (may be fractional, e.g. 0.5).",
								Optional:    true,
							},
						},
					},
				},
			},
		},
	}
}

func (r *RateLimitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

// ConfigValidators registers plan-time checks for the scope and algorithm
// tagged unions. Catching these here is much friendlier than letting the
// admin API return a validation error on apply.
func (r *RateLimitResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{exactlyOneAlgorithmValidator{}}
}

func (r *RateLimitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RateLimitResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	annotations := extractAnnotations(ctx, plan.Annotations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := buildRateLimitSpec(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build rate-limit spec", err.Error())
		return
	}

	rl, err := r.client.CreateRateLimit(ctx, client.CreateRateLimitRequest{
		APIVersion: client.RateLimitAPIVersion,
		Kind:       client.RateLimitKind,
		Metadata: client.RateLimitMetadata{
			Namespace:   plan.Namespace.ValueString(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create rate limit", err.Error())
		return
	}

	setRateLimitState(&plan, rl)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RateLimitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RateLimitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rl, err := r.client.GetRateLimit(ctx, state.Id.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read rate limit", err.Error())
		return
	}

	setRateLimitState(&state, rl)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RateLimitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RateLimitResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	annotations := extractAnnotations(ctx, plan.Annotations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := buildRateLimitSpec(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build rate-limit spec", err.Error())
		return
	}

	updateReq := client.UpdateRateLimitRequest{
		APIVersion: client.RateLimitAPIVersion,
		Kind:       client.RateLimitKind,
		Metadata:   &client.RateLimitMetadataPatch{},
		Spec: &client.RateLimitSpecPatch{
			Scope:     spec.Scope,
			Mode:      &spec.Mode,
			Selector:  &spec.Selector,
			Bucket:    &spec.Bucket,
			Algorithm: &spec.Algorithm,
		},
	}
	if labels != nil {
		updateReq.Metadata.Labels = &labels
	}
	if annotations != nil {
		updateReq.Metadata.Annotations = &annotations
	}

	rl, err := r.client.UpdateRateLimit(ctx, plan.Id.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update rate limit", err.Error())
		return
	}

	setRateLimitState(&plan, rl)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RateLimitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RateLimitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRateLimit(ctx, state.Id.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete rate limit", err.Error())
	}
}

func (r *RateLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("id"), req.ID)...)
}

// --- helpers ---

// buildRateLimitSpec projects the user's HCL model into the wire payload the
// admin API expects.
func buildRateLimitSpec(ctx context.Context, plan *RateLimitResourceModel) (client.RateLimitSpec, error) {
	def := client.RateLimitSpec{
		Mode: plan.Mode.ValueString(),
	}
	if plan.Scope != nil {
		def.Scope = &client.RateLimitScope{}
		def.Scope.NamespaceMatcher = plan.Scope.NamespaceMatcher.ValueString()
		if plan.Scope.ConnectorRef != nil {
			def.Scope.ConnectorRef = &client.ObjectReference{
				APIVersion: client.RateLimitAPIVersion,
				Kind:       "Connector",
				ID:         plan.Scope.ConnectorRef.ID.ValueString(),
				Generation: uint64(plan.Scope.ConnectorRef.Generation.ValueInt64()),
			}
		}
		if plan.Scope.ConnectionRef != nil {
			def.Scope.ConnectionRef = &client.ObjectReference{
				APIVersion: client.RateLimitAPIVersion,
				Kind:       "Connection",
				ID:         plan.Scope.ConnectionRef.ID.ValueString(),
			}
		}
	}

	if plan.Selector != nil {
		def.Selector.LabelSelector = plan.Selector.LabelSelector.ValueString()
		if methods, err := listToStrings(ctx, plan.Selector.Methods); err != nil {
			return def, fmt.Errorf("selector.methods: %w", err)
		} else {
			def.Selector.Methods = methods
		}
		if rts, err := listToStrings(ctx, plan.Selector.RequestTypes); err != nil {
			return def, fmt.Errorf("selector.request_types: %w", err)
		} else {
			def.Selector.RequestTypes = rts
		}
		if plan.Selector.PathMatch != nil {
			def.Selector.PathMatch = &client.RateLimitPathMatch{
				Kind:  plan.Selector.PathMatch.Kind.ValueString(),
				Value: plan.Selector.PathMatch.Value.ValueString(),
			}
		}
	}

	if plan.Bucket != nil {
		if dims, err := listToStrings(ctx, plan.Bucket.Dimensions); err != nil {
			return def, fmt.Errorf("bucket.dimensions: %w", err)
		} else {
			def.Bucket.Dimensions = dims
		}
	}

	if plan.Algorithm != nil {
		switch {
		case plan.Algorithm.FixedWindow != nil:
			def.Algorithm.FixedWindow = &client.RateLimitFixedWindow{
				Window: plan.Algorithm.FixedWindow.Window.ValueString(),
				Limit:  int(plan.Algorithm.FixedWindow.Limit.ValueInt64()),
			}
		case plan.Algorithm.SlidingWindow != nil:
			def.Algorithm.SlidingWindow = &client.RateLimitSlidingWindow{
				Window: plan.Algorithm.SlidingWindow.Window.ValueString(),
				Limit:  int(plan.Algorithm.SlidingWindow.Limit.ValueInt64()),
				Mode:   plan.Algorithm.SlidingWindow.Mode.ValueString(),
			}
		case plan.Algorithm.TokenBucket != nil:
			def.Algorithm.TokenBucket = &client.RateLimitTokenBucket{
				Capacity:   int(plan.Algorithm.TokenBucket.Capacity.ValueInt64()),
				RefillRate: plan.Algorithm.TokenBucket.RefillRate.ValueFloat64(),
			}
		}
	}

	return def, nil
}

// setRateLimitState populates a model from the API response, including
// the nested blocks. Used by all of Create/Read/Update.
func setRateLimitState(model *RateLimitResourceModel, rl *client.RateLimit) {
	model.Id = types.StringValue(rl.Metadata.ID)
	model.Namespace = types.StringValue(rl.Metadata.Namespace)
	if rl.Spec.Mode != "" {
		model.Mode = types.StringValue(rl.Spec.Mode)
	} else {
		// The server returns mode = "" for the default ("enforce");
		// surface that explicitly so plan/apply consistency holds.
		model.Mode = types.StringValue("enforce")
	}
	model.Labels = labelsToMap(rl.Metadata.Labels)
	model.Annotations = annotationsToMap(rl.Metadata.Annotations)
	if rl.Metadata.CreatedAt != nil {
		model.CreatedAt = types.StringValue(rl.Metadata.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if rl.Metadata.UpdatedAt != nil {
		model.UpdatedAt = types.StringValue(rl.Metadata.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	model.Scope = nil
	if rl.Spec.Scope != nil {
		model.Scope = &rateLimitScopeModel{NamespaceMatcher: types.StringNull()}
		if rl.Spec.Scope.NamespaceMatcher != "" {
			model.Scope.NamespaceMatcher = types.StringValue(rl.Spec.Scope.NamespaceMatcher)
		}
		if rl.Spec.Scope.ConnectorRef != nil {
			generation := types.Int64Null()
			if rl.Spec.Scope.ConnectorRef.Generation != 0 {
				generation = types.Int64Value(int64(rl.Spec.Scope.ConnectorRef.Generation))
			}
			model.Scope.ConnectorRef = &rateLimitConnectorRefModel{
				ID:         types.StringValue(rl.Spec.Scope.ConnectorRef.ID),
				Generation: generation,
			}
		}
		if rl.Spec.Scope.ConnectionRef != nil {
			model.Scope.ConnectionRef = &rateLimitConnectionRefModel{
				ID: types.StringValue(rl.Spec.Scope.ConnectionRef.ID),
			}
		}
	}

	model.Selector = &rateLimitSelectorModel{
		LabelSelector: optionalString(rl.Spec.Selector.LabelSelector),
		Methods:       stringsToList(rl.Spec.Selector.Methods),
		RequestTypes:  stringsToList(rl.Spec.Selector.RequestTypes),
	}
	if rl.Spec.Selector.PathMatch != nil {
		model.Selector.PathMatch = &rateLimitPathMatchModel{
			Kind:  types.StringValue(rl.Spec.Selector.PathMatch.Kind),
			Value: types.StringValue(rl.Spec.Selector.PathMatch.Value),
		}
	}

	model.Bucket = &rateLimitBucketModel{
		Dimensions: stringsToList(rl.Spec.Bucket.Dimensions),
	}

	algoModel := &rateLimitAlgorithmModel{}
	switch {
	case rl.Spec.Algorithm.FixedWindow != nil:
		algoModel.FixedWindow = &rateLimitFixedWindowModel{
			Window: types.StringValue(rl.Spec.Algorithm.FixedWindow.Window),
			Limit:  types.Int64Value(int64(rl.Spec.Algorithm.FixedWindow.Limit)),
		}
	case rl.Spec.Algorithm.SlidingWindow != nil:
		algoModel.SlidingWindow = &rateLimitSlidingWindowModel{
			Window: types.StringValue(rl.Spec.Algorithm.SlidingWindow.Window),
			Limit:  types.Int64Value(int64(rl.Spec.Algorithm.SlidingWindow.Limit)),
			Mode:   types.StringValue(rl.Spec.Algorithm.SlidingWindow.Mode),
		}
	case rl.Spec.Algorithm.TokenBucket != nil:
		algoModel.TokenBucket = &rateLimitTokenBucketModel{
			Capacity:   types.Int64Value(int64(rl.Spec.Algorithm.TokenBucket.Capacity)),
			RefillRate: types.Float64Value(rl.Spec.Algorithm.TokenBucket.RefillRate),
		}
	}
	model.Algorithm = algoModel
}

// optionalString returns a Null types.String for an empty input so
// optional attributes don't show as "" → null diffs.
func optionalString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// listToStrings extracts []string from a types.List. Returns nil for
// null/unknown so the JSON encoder omits the field (matching the
// server's omitempty contract).
func listToStrings(ctx context.Context, l types.List) ([]string, error) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	out := make([]string, 0, len(l.Elements()))
	diags := l.ElementsAs(ctx, &out, false)
	if diags.HasError() {
		return nil, fmt.Errorf("%s", diags.Errors())
	}
	return out, nil
}

// stringsToList renders a []string back as a types.List. Empty / nil
// becomes a Null list so plan and state line up when the API returns
// nothing.
func stringsToList(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	elements := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elements = append(elements, types.StringValue(s))
	}
	l, _ := types.ListValue(types.StringType, elements)
	return l
}

// --- algorithm exactly-one validator ---

type exactlyOneAlgorithmValidator struct{}

func (v exactlyOneAlgorithmValidator) Description(_ context.Context) string {
	return "ensures exactly one of algorithm.fixed_window / sliding_window / token_bucket is set"
}
func (v exactlyOneAlgorithmValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v exactlyOneAlgorithmValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model RateLimitResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if model.Scope != nil {
		set := 0
		if !model.Scope.NamespaceMatcher.IsNull() && !model.Scope.NamespaceMatcher.IsUnknown() && model.Scope.NamespaceMatcher.ValueString() != "" {
			set++
		}
		if model.Scope.ConnectorRef != nil {
			set++
		}
		if model.Scope.ConnectionRef != nil {
			set++
		}
		if set != 1 {
			resp.Diagnostics.AddAttributeError(
				path.Root("scope"),
				"Invalid rate-limit scope",
				"Exactly one of namespace_matcher, connector_ref, or connection_ref must be set when scope is configured.",
			)
		}
	}

	if model.Algorithm == nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("algorithm"),
			"Missing algorithm block",
			"Exactly one of fixed_window, sliding_window, or token_bucket must be set.",
		)
		return
	}

	set := 0
	if model.Algorithm.FixedWindow != nil {
		set++
	}
	if model.Algorithm.SlidingWindow != nil {
		set++
	}
	if model.Algorithm.TokenBucket != nil {
		set++
	}

	switch set {
	case 0:
		resp.Diagnostics.AddAttributeError(
			path.Root("algorithm"),
			"No algorithm variant set",
			"Exactly one of fixed_window, sliding_window, or token_bucket must be set.",
		)
	case 1:
		// ok
	default:
		resp.Diagnostics.AddAttributeError(
			path.Root("algorithm"),
			"Multiple algorithm variants set",
			fmt.Sprintf("Exactly one of fixed_window, sliding_window, or token_bucket must be set; %d are configured.", set),
		)
	}
}
