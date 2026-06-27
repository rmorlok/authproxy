import {describe, expect, it} from 'vitest';
import {
  buildKeyDataPayload,
  createEmptyKeyDataFormState,
  keyDataFormStateFromConfig,
  validateKeyDataFormState,
} from './KeyDataForm';

describe('KeyDataForm helpers', () => {
  it('builds random key data with the provider discriminator', () => {
    expect(buildKeyDataPayload(createEmptyKeyDataFormState())).toEqual({
      random: true,
      num_bytes: 32,
    });
  });

  it('builds AWS KMS key data with explicit credentials', () => {
    const payload = buildKeyDataPayload({
      type: 'aws_kms',
      fields: {
        aws_kms_key_id: 'alias/authproxy',
        aws_region: 'us-east-1',
        aws_kms_endpoint: 'http://localhost:4566',
        cache_ttl: '5m',
        aws_credentials_type: 'access_key',
        aws_access_key_id: 'test-access-key',
        aws_secret_access_key: 'test-secret-key',
      },
    });

    expect(payload).toEqual({
      aws_kms_key_id: 'alias/authproxy',
      aws_region: 'us-east-1',
      aws_kms_endpoint: 'http://localhost:4566',
      cache_ttl: '5m',
      aws_credentials: {
        type: 'access_key',
        access_key_id: 'test-access-key',
        secret_access_key: 'test-secret-key',
      },
    });
  });

  it('builds GCP KMS key data from split resource fields', () => {
    const payload = buildKeyDataPayload({
      type: 'gcp_kms',
      fields: {
        gcp_project: 'auth-proxy-446804',
        gcp_location: 'global',
        gcp_key_ring: 'authproxy',
        gcp_crypto_key: 'authproxy-github-actions-ci-1',
        gcp_credentials_file: '/tmp/gcp.json',
        cache_ttl: '5m',
      },
    });

    expect(payload).toEqual({
      gcp_project: 'auth-proxy-446804',
      gcp_location: 'global',
      gcp_key_ring: 'authproxy',
      gcp_crypto_key: 'authproxy-github-actions-ci-1',
      gcp_credentials_file: '/tmp/gcp.json',
      cache_ttl: '5m',
    });
  });

  it('round-trips API summaries into editable state', () => {
    expect(keyDataFormStateFromConfig({
      type: 'aws_kms',
      fields: {
        aws_kms_key_id: 'alias/authproxy',
        aws_region: 'us-east-1',
        aws_credentials_type: 'implicit',
      },
      sensitive_fields: ['aws_access_key_id', 'aws_secret_access_key'],
    })).toEqual({
      type: 'aws_kms',
      fields: {
        num_bytes: '32',
        aws_credentials_type: 'implicit',
        aws_kms_key_id: 'alias/authproxy',
        aws_region: 'us-east-1',
      },
    });
  });

  it('validates cloud provider required fields', () => {
    expect(validateKeyDataFormState({
      type: 'gcp_kms',
      fields: {
        gcp_project: 'auth-proxy-446804',
        gcp_location: 'global',
      },
    })).toBe('GCP Key Ring is required');

    expect(validateKeyDataFormState({
      type: 'aws_kms',
      fields: {
        aws_kms_key_id: 'alias/authproxy',
        aws_credentials_type: 'access_key',
        aws_access_key_id: 'test-access-key',
      },
    })).toBe('AWS Secret Access Key is required');
  });
});
