package openapi

import (
	"encoding/json"
	"time"

	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ResourceListJson is the non-generic OpenAPI projection of the common list
// transport. Concrete list adapters embed it and specialize Items because
// swaggo cannot reliably document instantiated generic Go types.
type ResourceListJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required"`
	Metadata   apiv1alpha1.ListMeta `json:"metadata" binding:"required"`
}

// ActorSpecJson documents Actor desired state while keeping polymorphic
// write-only signing-key configuration opaque to swaggo.
type ActorSpecJson struct {
	ExternalId  string                  `json:"externalId" binding:"required" example:"user-123"`
	Permissions []authschema.Permission `json:"permissions,omitempty"`
	SigningKey  map[string]interface{}  `json:"signingKey,omitempty" swaggertype:"object"`
}

// ActorJson documents a canonical Actor resource.
//
//	@Description	Kubernetes-style Actor resource. Signing key material is write-only and never returned.
type ActorJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"Actor" example:"Actor"`
	Metadata   meta.ObjectMeta          `json:"metadata" binding:"required"`
	Spec       ActorSpecJson            `json:"spec" binding:"required"`
	Status     *actorschema.ActorStatus `json:"status,omitempty"`
}

// ActorSpecPatchJson documents mutable Actor desired state.
type ActorSpecPatchJson struct {
	ExternalId  *string                  `json:"externalId,omitempty" example:"user-123"`
	Permissions *[]authschema.Permission `json:"permissions,omitempty"`
	SigningKey  map[string]interface{}   `json:"signingKey,omitempty" swaggertype:"object"`
}

// ActorPatchJson documents a canonical Actor update.
//
//	@Description	Kubernetes-style Actor patch. External identity and namespace are immutable; signingKey null removes actor-specific signing material.
type ActorPatchJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"Actor" example:"Actor"`
	Metadata   *meta.ObjectMetaPatch    `json:"metadata" binding:"required"`
	Spec       *ActorSpecPatchJson      `json:"spec" binding:"required"`
	Status     *actorschema.ActorStatus `json:"status,omitempty"`
}

// ListActorsResponseJson documents the paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	ResourceListJson
	// List of actors.
	Items []ActorJson `json:"items" binding:"required"`
}

// ListNamespacesResponseJson documents the paginated namespace list response.
//
//	@Description	Paginated list of namespaces
type ListNamespacesResponseJson struct {
	ResourceListJson
	// List of namespaces.
	Items []nschema.Namespace `json:"items" binding:"required"`
}

// ConnectorSpecJson documents connector desired state while keeping the
// polymorphic provider definition opaque to swaggo.
type ConnectorSpecJson struct {
	Release    cschema.ConnectorReleaseSpec `json:"release,omitempty"`
	Definition map[string]interface{}       `json:"definition" swaggertype:"object"`
}

// ConnectorJson documents one logical connector generation. Every connector
// endpoint uses this same kind; metadata.generation selects the generation.
//
//	@Description	Kubernetes-style Connector resource
type ConnectorJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"Connector" example:"Connector"`
	Metadata   meta.ObjectMeta          `json:"metadata" binding:"required"`
	Spec       ConnectorSpecJson        `json:"spec" binding:"required"`
	Status     *cschema.ConnectorStatus `json:"status,omitempty"`
}

// ConnectorReleaseSpecPatchJson documents desired release-state updates.
type ConnectorReleaseSpecPatchJson struct {
	DesiredState *cschema.ConnectorReleaseState `json:"desiredState,omitempty" enums:"draft,primary"`
}

// ConnectorSpecPatchJson documents mutable connector desired state.
type ConnectorSpecPatchJson struct {
	Release    *ConnectorReleaseSpecPatchJson `json:"release,omitempty"`
	Definition map[string]interface{}         `json:"definition,omitempty" swaggertype:"object"`
}

// ConnectorPatchJson documents a canonical Connector update.
//
//	@Description	Kubernetes-style Connector patch
type ConnectorPatchJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"Connector" example:"Connector"`
	Metadata   *meta.ObjectMetaPatch    `json:"metadata" binding:"required"`
	Spec       *ConnectorSpecPatchJson  `json:"spec" binding:"required"`
	Status     *cschema.ConnectorStatus `json:"status,omitempty"`
}

// ListConnectorsResponseJson documents the paginated connector list response.
//
//	@Description	Paginated list of connectors
type ListConnectorsResponseJson struct {
	ResourceListJson
	Items []ConnectorJson `json:"items" binding:"required"`
}

// ListConnectorGenerationsResponseJson documents the paginated connector generation list response.
//
//	@Description	Paginated list of connector generations
type ListConnectorGenerationsResponseJson struct {
	ResourceListJson
	Items []ConnectorJson `json:"items" binding:"required"`
}

// ConnectionSpecJson documents the durable references and connector-authored
// configuration for a Connection.
type ConnectionSpecJson struct {
	ConnectorRef  meta.ObjectReference   `json:"connectorRef" binding:"required"`
	Configuration map[string]interface{} `json:"configuration,omitempty" swaggertype:"object"`
}

// ConnectionJson documents a canonical Connection resource.
//
//	@Description	Kubernetes-style Connection resource
type ConnectionJson struct {
	APIVersion string                             `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                             `json:"kind" binding:"required" enums:"Connection" example:"Connection"`
	Metadata   meta.ObjectMeta                    `json:"metadata" binding:"required"`
	Spec       ConnectionSpecJson                 `json:"spec" binding:"required"`
	Status     *connectionschema.ConnectionStatus `json:"status,omitempty"`
}

// ConnectionPatchJson documents metadata-only Connection updates.
//
//	@Description	Kubernetes-style Connection patch. Connector and actor bindings are immutable.
type ConnectionPatchJson struct {
	APIVersion string                                `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                                `json:"kind" binding:"required" enums:"Connection" example:"Connection"`
	Metadata   *meta.ObjectMetaPatch                 `json:"metadata" binding:"required"`
	Spec       *connectionschema.ConnectionSpecPatch `json:"spec" binding:"required"`
	Status     *connectionschema.ConnectionStatus    `json:"status,omitempty"`
}

// ListConnectionResponseJson documents the paginated connection list response.
//
//	@Description	Paginated list of connections
type ListConnectionResponseJson struct {
	ResourceListJson
	Items []ConnectionJson `json:"items" binding:"required"`
}

// DataSourceOptionListJson documents dynamic options using the standard list
// envelope.
type DataSourceOptionListJson struct {
	ResourceListJson
	Items []schemaapi.DataSourceOptionJson `json:"items" binding:"required"`
}

// ConnectionScopeListJson documents requested and granted OAuth2 scopes using
// the standard list envelope.
type ConnectionScopeListJson struct {
	ResourceListJson
	Items []schemaapi.ConnectionScopeJson `json:"items" binding:"required"`
}

// ConnectionActionMetaJson identifies the resource targeted by an action.
type ConnectionActionMetaJson struct {
	Target meta.ObjectReference `json:"target" binding:"required"`
}

// ConnectionInitiateActionJson documents a request to create and begin setup
// for a Connection. metadata.target identifies the Connector to use.
type ConnectionInitiateActionJson struct {
	APIVersion string                           `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                           `json:"kind" binding:"required" enums:"ConnectionInitiate" example:"ConnectionInitiate"`
	Metadata   ConnectionActionMetaJson         `json:"metadata" binding:"required"`
	Spec       schemaapi.ConnectionInitiateSpec `json:"spec" binding:"required"`
}

// ConnectionSetupActionJson documents the observed next setup step.
type ConnectionSetupActionJson struct {
	APIVersion string                           `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                           `json:"kind" binding:"required" enums:"ConnectionSetup" example:"ConnectionSetup"`
	Metadata   ConnectionActionMetaJson         `json:"metadata" binding:"required"`
	Spec       struct{}                         `json:"spec" binding:"required"`
	Status     *ConnectionSetupActionStatusJson `json:"status,omitempty"`
}

type ConnectionSetupActionStatusJson struct {
	Type            string          `json:"type" enums:"redirect,form,complete,verifying,error"`
	RedirectURL     string          `json:"redirectUrl,omitempty"`
	StepID          string          `json:"stepId,omitempty"`
	StepTitle       string          `json:"stepTitle,omitempty"`
	StepDescription string          `json:"stepDescription,omitempty"`
	JSONSchema      json.RawMessage `json:"jsonSchema,omitempty" swaggertype:"object"`
	UISchema        json.RawMessage `json:"uiSchema,omitempty" swaggertype:"object"`
	Data            json.RawMessage `json:"data,omitempty" swaggertype:"object"`
	Error           string          `json:"error,omitempty"`
	CanRetry        bool            `json:"canRetry,omitempty"`
}

type ConnectionSetupSubmitSpecJson struct {
	StepID      string                 `json:"stepId" binding:"required"`
	Data        map[string]interface{} `json:"data" binding:"required"`
	ReturnToURL string                 `json:"returnToUrl,omitempty"`
}

type ConnectionSetupSubmitActionJson struct {
	APIVersion string                        `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                        `json:"kind" binding:"required" enums:"ConnectionSetupSubmit" example:"ConnectionSetupSubmit"`
	Metadata   ConnectionActionMetaJson      `json:"metadata" binding:"required"`
	Spec       ConnectionSetupSubmitSpecJson `json:"spec" binding:"required"`
}

// ConnectionSetupControlActionJson documents retry and reauthentication
// request bodies, whose kinds distinguish the requested operation.
type ConnectionSetupControlActionJson struct {
	APIVersion string                               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                               `json:"kind" binding:"required" enums:"ConnectionSetupRetry,ConnectionReauthenticate"`
	Metadata   ConnectionActionMetaJson             `json:"metadata" binding:"required"`
	Spec       schemaapi.ConnectionSetupControlSpec `json:"spec" binding:"required"`
}

// EmptyConnectionActionJson documents setup actions with no parameters.
type EmptyConnectionActionJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"ConnectionSetupAbort,ConnectionReconfigure,ConnectionSetupCancel"`
	Metadata   ConnectionActionMetaJson `json:"metadata" binding:"required"`
	Spec       struct{}                 `json:"spec" binding:"required"`
}

type ConnectionDisconnectSpecJson struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" example:"600"`
}

type ConnectionDisconnectStatusJson struct {
	TaskID     string         `json:"taskId"`
	Connection ConnectionJson `json:"connection"`
}

type ConnectionDisconnectActionJson struct {
	APIVersion string                          `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                          `json:"kind" binding:"required" enums:"ConnectionDisconnect" example:"ConnectionDisconnect"`
	Metadata   ConnectionActionMetaJson        `json:"metadata" binding:"required"`
	Spec       ConnectionDisconnectSpecJson    `json:"spec" binding:"required"`
	Status     *ConnectionDisconnectStatusJson `json:"status,omitempty"`
}

type ConnectionGenerationMigrationSpecJson struct {
	ConnectorRef   meta.ObjectReference `json:"connectorRef" binding:"required"`
	TimeoutSeconds *int64               `json:"timeoutSeconds,omitempty" example:"600"`
}

type ConnectionGenerationMigrationStatusJson struct {
	TaskID             string               `json:"taskId"`
	SourceConnectorRef meta.ObjectReference `json:"sourceConnectorRef"`
	TargetConnectorRef meta.ObjectReference `json:"targetConnectorRef"`
}

type ConnectionGenerationMigrationActionJson struct {
	APIVersion string                                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                                   `json:"kind" binding:"required" enums:"ConnectionGenerationMigration" example:"ConnectionGenerationMigration"`
	Metadata   ConnectionActionMetaJson                 `json:"metadata" binding:"required"`
	Spec       ConnectionGenerationMigrationSpecJson    `json:"spec" binding:"required"`
	Status     *ConnectionGenerationMigrationStatusJson `json:"status,omitempty"`
}

type ConnectionForceStateSpecJson struct {
	State string `json:"state" binding:"required" enums:"setup,configured,disabled,disconnecting,disconnected"`
}

type ConnectionForceStateStatusJson struct {
	Connection ConnectionJson `json:"connection"`
}

type ConnectionForceStateActionJson struct {
	APIVersion string                          `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                          `json:"kind" binding:"required" enums:"ConnectionForceState" example:"ConnectionForceState"`
	Metadata   ConnectionActionMetaJson        `json:"metadata" binding:"required"`
	Spec       ConnectionForceStateSpecJson    `json:"spec" binding:"required"`
	Status     *ConnectionForceStateStatusJson `json:"status,omitempty"`
}

type NotificationSpecJson struct {
	Key         string                 `json:"key" binding:"required"`
	Level       string                 `json:"level" binding:"required" enums:"info,warning,error" example:"warning"`
	ResourceRef meta.ObjectReference   `json:"resourceRef" binding:"required"`
	Title       string                 `json:"title" binding:"required"`
	Message     string                 `json:"message" binding:"required"`
	Context     map[string]interface{} `json:"context,omitempty" swaggertype:"object"`
}

type NotificationActionStatusJson struct {
	URL string `json:"url" binding:"required"`
}

type NotificationStatusJson struct {
	State      string                        `json:"state" binding:"required" enums:"active,resolved" example:"active"`
	Viewed     bool                          `json:"viewed"`
	Action     *NotificationActionStatusJson `json:"action,omitempty"`
	ResolvedAt *time.Time                    `json:"resolvedAt,omitempty"`
}

// NotificationJson documents an actor-visible resource-shaped projection.
//
//	@Description	Actor-visible, resource-shaped notification projection
type NotificationJson struct {
	APIVersion string                 `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                 `json:"kind" binding:"required" enums:"Notification" example:"Notification"`
	Metadata   meta.ObjectMeta        `json:"metadata" binding:"required"`
	Spec       NotificationSpecJson   `json:"spec" binding:"required"`
	Status     NotificationStatusJson `json:"status" binding:"required"`
}

// ListNotificationsResponseJson documents the paginated notification list response.
//
//	@Description	Paginated list of actor-visible notifications
type ListNotificationsResponseJson struct {
	ResourceListJson
	Items []NotificationJson `json:"items" binding:"required"`
}

type NotificationViewSpecJson struct{}

type NotificationViewStatusJson struct {
	Viewed bool `json:"viewed"`
}

type NotificationViewActionJson struct {
	APIVersion string                      `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                      `json:"kind" binding:"required" enums:"NotificationView" example:"NotificationView"`
	Metadata   apiv1alpha1.ActionMeta      `json:"metadata" binding:"required"`
	Spec       NotificationViewSpecJson    `json:"spec" binding:"required"`
	Status     *NotificationViewStatusJson `json:"status,omitempty"`
}

type NotificationBatchViewMetadataJson struct {
	Targets []meta.ObjectReference `json:"targets" binding:"required"`
}

type NotificationBatchViewStatusJson struct {
	ViewedCount int `json:"viewedCount"`
}

type NotificationBatchViewActionJson struct {
	APIVersion string                            `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                            `json:"kind" binding:"required" enums:"NotificationBatchView" example:"NotificationBatchView"`
	Metadata   NotificationBatchViewMetadataJson `json:"metadata" binding:"required"`
	Spec       NotificationViewSpecJson          `json:"spec" binding:"required"`
	Status     *NotificationBatchViewStatusJson  `json:"status,omitempty"`
}

// ConnectorLifecycleActionJson documents connector-wide lifecycle actions.
//
//	@Description	Kubernetes-style connector lifecycle action and task result
type ConnectorLifecycleActionJson struct {
	APIVersion string                              `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                              `json:"kind" binding:"required" enums:"ConnectorDisconnectAll,ConnectorArchive"`
	Metadata   ConnectionActionMetaJson            `json:"metadata" binding:"required"`
	Spec       schemaapi.ConnectorLifecycleSpec    `json:"spec" binding:"required"`
	Status     *schemaapi.ConnectorLifecycleStatus `json:"status,omitempty"`
}

// ConnectorForceStateActionJson documents an exact-generation force-state
// request. The endpoint returns the updated Connector resource.
//
//	@Description	Kubernetes-style connector generation force-state action
type ConnectorForceStateActionJson struct {
	APIVersion string                            `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                            `json:"kind" binding:"required" enums:"ConnectorForceState" example:"ConnectorForceState"`
	Metadata   ConnectionActionMetaJson          `json:"metadata" binding:"required"`
	Spec       schemaapi.ConnectorForceStateSpec `json:"spec" binding:"required"`
}

// KeySpecJson documents managed-key desired state while keeping polymorphic
// provider configuration opaque to swaggo. No key-material examples belong in
// this projection.
type KeySpecJson struct {
	Usage        keyschema.KeyUsage        `json:"usage,omitempty" enums:"data_encryption"`
	MaterialType keyschema.KeyMaterialType `json:"materialType,omitempty" enums:"symmetric,public,private,external"`
	DesiredState keyschema.KeyState        `json:"desiredState,omitempty" enums:"active,disabled"`
	KeyData      map[string]interface{}    `json:"keyData,omitempty" swaggertype:"object"`
}

// KeyJson documents a managed key resource response.
//
//	@Description	Kubernetes-style managed Key resource. Secret keyData fields are always redacted in responses.
type KeyJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required" enums:"Key" example:"Key"`
	Metadata   meta.ObjectMeta      `json:"metadata" binding:"required"`
	Spec       KeySpecJson          `json:"spec" binding:"required"`
	Status     *keyschema.KeyStatus `json:"status,omitempty"`
}

// ListKeysResponseJson documents the paginated key list response.
//
//	@Description	Paginated list of keys
type ListKeysResponseJson struct {
	ResourceListJson
	Items []KeyJson `json:"items" binding:"required"`
}

// MetricsSchemaResponseJson documents the metrics schema response.
//
//	@Description	Application metrics schema response
type MetricsSchemaResponseJson struct {
	Metrics []schemaapi.MetricsSchemaMetricJson `json:"metrics"`
}

// TaskJson documents public background task status.
//
//	@Description	Background task status
type TaskJson struct {
	APIVersion string          `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string          `json:"kind" binding:"required" enums:"Task" example:"Task"`
	Metadata   meta.ObjectMeta `json:"metadata" binding:"required"`
	Spec       TaskSpecJson    `json:"spec" binding:"required"`
	Status     TaskStatusJson  `json:"status" binding:"required"`
}

type TaskSpecJson struct {
	Type string `json:"type"`
}

type TaskStatusJson struct {
	State string `json:"state" example:"completed" enums:"unknown,active,pending,scheduled,retry,failed,completed"`
}

// KeySpecPatchJson documents mutable desired key fields.
type KeySpecPatchJson struct {
	Usage        *keyschema.KeyUsage        `json:"usage,omitempty" enums:"data_encryption"`
	MaterialType *keyschema.KeyMaterialType `json:"materialType,omitempty" enums:"symmetric,public,private,external"`
	DesiredState *keyschema.KeyState        `json:"desiredState,omitempty" enums:"active,disabled"`
	KeyData      map[string]interface{}     `json:"keyData,omitempty" swaggertype:"object"`
}

// KeyPatchJson documents the managed-key update body.
//
//	@Description	Kubernetes-style managed Key patch. Status and server-owned metadata are rejected.
type KeyPatchJson struct {
	APIVersion string                `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                `json:"kind" binding:"required" enums:"Key" example:"Key"`
	Metadata   *meta.ObjectMetaPatch `json:"metadata" binding:"required"`
	Spec       *KeySpecPatchJson     `json:"spec" binding:"required"`
	Status     *keyschema.KeyStatus  `json:"status,omitempty"`
}

type RateLimitPathMatchJson struct {
	Kind  string `json:"kind" enums:"prefix,glob,regex"`
	Value string `json:"value"`
}

type RateLimitSelectorJson struct {
	LabelSelector string                  `json:"labelSelector,omitempty"`
	Methods       []string                `json:"methods,omitempty"`
	PathMatch     *RateLimitPathMatchJson `json:"pathMatch,omitempty"`
	RequestTypes  []string                `json:"requestTypes,omitempty" enums:"global,proxy,oauth,public,probe"`
}

type RateLimitBucketJson struct {
	Dimensions []string `json:"dimensions,omitempty"`
}

type RateLimitFixedWindowJson struct {
	Window string `json:"window" example:"1m"`
	Limit  int    `json:"limit" example:"100"`
}

type RateLimitSlidingWindowJson struct {
	Window string `json:"window" example:"1m"`
	Limit  int    `json:"limit" example:"100"`
	Mode   string `json:"mode" enums:"log,counter"`
}

type RateLimitTokenBucketJson struct {
	Capacity   int     `json:"capacity" example:"60"`
	RefillRate float64 `json:"refillRate" example:"1"`
}

type RateLimitAlgorithmJson struct {
	FixedWindow   *RateLimitFixedWindowJson   `json:"fixedWindow,omitempty"`
	SlidingWindow *RateLimitSlidingWindowJson `json:"slidingWindow,omitempty"`
	TokenBucket   *RateLimitTokenBucketJson   `json:"tokenBucket,omitempty"`
}

// RateLimitConnectorReferenceJson documents a generationless reference to a
// Connector. Rate limits bind to connector identity, not one generation.
type RateLimitConnectorReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Connector" example:"Connector"`
	ID         string `json:"id,omitempty" example:"cxr_01example"`
	Name       string `json:"name,omitempty" example:"salesforce"`
	Namespace  string `json:"namespace,omitempty" example:"root.acme"`
}

type RateLimitConnectionReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Connection" example:"Connection"`
	ID         string `json:"id,omitempty" example:"cxn_01example"`
	Name       string `json:"name,omitempty" example:"salesforce-production"`
	Namespace  string `json:"namespace,omitempty" example:"root.acme"`
}

type RateLimitScopeJson struct {
	NamespaceMatcher *string                           `json:"namespaceMatcher,omitempty" example:"root.acme.payments.**"`
	ConnectorRef     *RateLimitConnectorReferenceJson  `json:"connectorRef,omitempty"`
	ConnectionRef    *RateLimitConnectionReferenceJson `json:"connectionRef,omitempty"`
}

type RateLimitSpecJson struct {
	Scope     *RateLimitScopeJson    `json:"scope,omitempty"`
	Mode      string                 `json:"mode,omitempty" enums:"enforce,observe"`
	Selector  RateLimitSelectorJson  `json:"selector" binding:"required"`
	Bucket    RateLimitBucketJson    `json:"bucket" binding:"required"`
	Algorithm RateLimitAlgorithmJson `json:"algorithm" binding:"required"`
}

type RateLimitStatusJson struct {
	EffectiveMode string `json:"effectiveMode" enums:"enforce,observe"`
}

// RateLimitJson documents a canonical rate-limit resource without exposing
// runtime-only schema implementations to swaggo's dependency walker.
//
//	@Description	Kubernetes-style RateLimit resource
type RateLimitJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required" enums:"RateLimit" example:"RateLimit"`
	Metadata   meta.ObjectMeta      `json:"metadata" binding:"required"`
	Spec       RateLimitSpecJson    `json:"spec" binding:"required"`
	Status     *RateLimitStatusJson `json:"status,omitempty"`
}

// ListRateLimitsResponseJson documents the paginated rate-limit list response.
//
//	@Description	Paginated list of rate limits
type ListRateLimitsResponseJson struct {
	ResourceListJson
	Items []RateLimitJson `json:"items" binding:"required"`
}

// RateLimitSpecPatchJson documents mutable desired rate-limit policy.
type RateLimitSpecPatchJson struct {
	Scope     *RateLimitScopeJson     `json:"scope,omitempty"`
	Mode      *string                 `json:"mode,omitempty" enums:"enforce,observe"`
	Selector  *RateLimitSelectorJson  `json:"selector,omitempty"`
	Bucket    *RateLimitBucketJson    `json:"bucket,omitempty"`
	Algorithm *RateLimitAlgorithmJson `json:"algorithm,omitempty"`
}

// RateLimitPatchJson documents a canonical rate-limit update.
//
//	@Description	Kubernetes-style RateLimit patch. Status and server-owned metadata are rejected.
type RateLimitPatchJson struct {
	APIVersion string                  `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                  `json:"kind" binding:"required" enums:"RateLimit" example:"RateLimit"`
	Metadata   *meta.ObjectMetaPatch   `json:"metadata" binding:"required"`
	Spec       *RateLimitSpecPatchJson `json:"spec" binding:"required"`
	Status     *RateLimitStatusJson    `json:"status,omitempty"`
}

// ProxyRequestJson documents the proxy-shaped request used by dry-run.
//
//	@Description	Request to proxy or simulate an HTTP request
type ProxyRequestJson struct {
	URL      string            `json:"url" example:"https://api.example.com/v1/users"`
	Method   string            `json:"method" example:"POST"`
	Headers  map[string]any    `json:"headers,omitempty" swaggertype:"object"`
	Labels   map[string]string `json:"labels,omitempty"`
	BodyRaw  []byte            `json:"bodyRaw,omitempty"`
	BodyJson interface{}       `json:"bodyJson,omitempty"`
}

// RateLimitDryRunSpecJson documents the synthetic request and optional actor
// used by a dry-run action. metadata.target identifies a Connection or
// Namespace.
//
//	@Description	Synthetic request evaluated by a rate-limit dry run
type RateLimitDryRunSpecJson struct {
	Request     ProxyRequestJson      `json:"request" binding:"required"`
	RequestType string                `json:"requestType" binding:"required" example:"proxy"`
	ActorRef    *meta.ObjectReference `json:"actorRef,omitempty"`
}

// RateLimitDryRunStatusJson documents the observed dry-run result.
//
//	@Description	Per-rule match and peek-driven would-allow result
type RateLimitDryRunStatusJson struct {
	RequestLabelSnapshot map[string]string      `json:"requestLabelSnapshot"`
	Matched              []DryRunMatchJson      `json:"matched"`
	NotMatched           []DryRunNotMatchedJson `json:"notMatched"`
}

// RateLimitDryRunActionJson documents the complete dry-run request/response
// envelope.
//
//	@Description	Kubernetes-style rate-limit dry-run action
type RateLimitDryRunActionJson struct {
	APIVersion string                     `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                     `json:"kind" binding:"required" enums:"RateLimitDryRun" example:"RateLimitDryRun"`
	Metadata   ConnectionActionMetaJson   `json:"metadata" binding:"required"`
	Spec       RateLimitDryRunSpecJson    `json:"spec" binding:"required"`
	Status     *RateLimitDryRunStatusJson `json:"status,omitempty"`
}

type DryRunMatchJson struct {
	RateLimitId      string `json:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string `json:"namespace" example:"root.acme"`
	EffectiveMode    string `json:"effectiveMode" example:"enforce"`
	BucketKey        string `json:"bucketKey" example:"actor=act_abc|labels/team=acme"`
	AlgorithmSummary string `json:"algorithmSummary" example:"token bucket 60 @ 1/s"`
	WouldAllow       bool   `json:"wouldAllow"`
	Remaining        int    `json:"remaining"`
	RetryAfterMs     int64  `json:"retryAfterMs"`
	PeekFailed       bool   `json:"peekFailed"`
}

type DryRunNotMatchedJson struct {
	RateLimitId string `json:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string `json:"namespace" example:"root.acme"`
	Reason      string `json:"reason"`
}

// ProxyResponseJson documents the response from a proxied request.
//
//	@Description	Response from a proxied HTTP request
type ProxyResponseJson struct {
	StatusCode int               `json:"statusCode" example:"200"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyRaw    []byte            `json:"bodyRaw,omitempty"`
	BodyJson   interface{}       `json:"bodyJson,omitempty"`
}
