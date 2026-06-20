package encrypt

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const (
	encryptTelemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/encrypt"

	metricDataEncryptionKeyGenerationFailures = "authproxy.encrypt.dek_generation.failures"

	attrDEKGenerationFailureReason = "authproxy.encrypt.failure_reason"
	attrDEKGenerationKeyScope      = "authproxy.encrypt.key_scope"
	attrKeyUsage                   = "authproxy.key.usage"
	attrKeyMaterialType            = "authproxy.key.material_type"
	attrKeyState                   = "authproxy.key.state"
	attrKeyProviderType            = "authproxy.key.provider_type"

	dekGenerationKeyScopeGlobal    = "global"
	dekGenerationKeyScopeNonGlobal = "non_global"

	dekGenerationProviderUnknown = "unknown"

	dekGenerationFailureReasonGlobalReconcile    = "global_reconcile_failed"
	dekGenerationFailureReasonGlobalCache        = "global_cache_failed"
	dekGenerationFailureReasonMissingWrapping    = "missing_wrapping_material"
	dekGenerationFailureReasonDecodeEncryptedKey = "decode_encrypted_key_data_failed"
	dekGenerationFailureReasonDecryptKeyData     = "decrypt_key_data_failed"
	dekGenerationFailureReasonUnmarshalKeyData   = "unmarshal_key_data_failed"
	dekGenerationFailureReasonReconcile          = "reconcile_dek_failed"
	dekGenerationFailureReasonCache              = "cache_dek_failed"
)

// DataEncryptionKeyTelemetry owns OTel metrics emitted by the encrypt package.
// A zero-value or nil instance is safe and emits nothing.
type DataEncryptionKeyTelemetry struct {
	metricsEnabled bool

	dekGenerationFailures metric.Int64Counter
}

func NewDataEncryptionKeyTelemetry(
	providers *aptelemetry.Providers,
	cfg *sconfig.Telemetry,
) (*DataEncryptionKeyTelemetry, error) {
	t := &DataEncryptionKeyTelemetry{}
	if providers == nil || !providers.Enabled || !cfg.MetricsEnabled() || providers.MeterProvider == nil {
		return t, nil
	}

	meter := providers.MeterProvider.Meter(encryptTelemetryInstrumentationName)
	failures, err := meter.Int64Counter(
		metricDataEncryptionKeyGenerationFailures,
		metric.WithUnit("{failure}"),
		metric.WithDescription("Number of data encryption key generation or reconciliation failures encountered while walking keys."),
	)
	if err != nil {
		return nil, fmt.Errorf("encrypt: create DEK generation failures counter: %w", err)
	}

	t.metricsEnabled = true
	t.dekGenerationFailures = failures
	return t, nil
}

func (t *DataEncryptionKeyTelemetry) recordDEKGenerationFailure(
	ctx context.Context,
	reason string,
	key *database.Key,
	provider sconfig.ProviderType,
) {
	if t == nil || !t.metricsEnabled {
		return
	}

	t.dekGenerationFailures.Add(ctx, 1, metric.WithAttributes(
		attribute.String(attrDEKGenerationFailureReason, reason),
		attribute.String(attrDEKGenerationKeyScope, dekGenerationKeyScope(key)),
		attribute.String(attrKeyUsage, boundedString(stringFromKey(key, func(k *database.Key) string { return string(k.Usage) }))),
		attribute.String(attrKeyMaterialType, boundedString(stringFromKey(key, func(k *database.Key) string { return string(k.MaterialType) }))),
		attribute.String(attrKeyState, boundedString(stringFromKey(key, func(k *database.Key) string { return string(k.State) }))),
		attribute.String(attrKeyProviderType, boundedProvider(provider)),
	))
}

func dekGenerationKeyScope(key *database.Key) string {
	if key != nil && key.Id == globalEncryptionKeyID {
		return dekGenerationKeyScopeGlobal
	}
	return dekGenerationKeyScopeNonGlobal
}

func boundedProvider(provider sconfig.ProviderType) string {
	if provider == "" {
		return dekGenerationProviderUnknown
	}
	return string(provider)
}

func boundedString(value string) string {
	if value == "" {
		return dekGenerationProviderUnknown
	}
	return value
}

func stringFromKey(key *database.Key, f func(*database.Key) string) string {
	if key == nil {
		return ""
	}
	return f(key)
}

type generateDataEncryptionKeysOptions struct {
	telemetry *DataEncryptionKeyTelemetry
}

type GenerateDataEncryptionKeysOption func(*generateDataEncryptionKeysOptions)

func WithGenerateDataEncryptionKeysTelemetry(tel *DataEncryptionKeyTelemetry) GenerateDataEncryptionKeysOption {
	return func(opts *generateDataEncryptionKeysOptions) {
		opts.telemetry = tel
	}
}

func newGenerateDataEncryptionKeysOptions(opts []GenerateDataEncryptionKeysOption) generateDataEncryptionKeysOptions {
	var out generateDataEncryptionKeysOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}
