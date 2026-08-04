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
      numBytes: 32,
    });
  });

  it('builds AWS KMS key data with explicit credentials', () => {
    const payload = buildKeyDataPayload({
      type: 'aws_kms',
      fields: {
        awsKmsKeyId: 'alias/authproxy',
        awsRegion: 'us-east-1',
        awsKmsEndpoint: 'http://localhost:4566',
        cacheTtl: '5m',
        awsCredentialsType: 'access_key',
        awsAccessKeyId: 'test-access-key',
        awsSecretAccessKey: 'test-secret-key',
      },
    });

    expect(payload).toEqual({
      awsKmsKeyId: 'alias/authproxy',
      awsRegion: 'us-east-1',
      awsKmsEndpoint: 'http://localhost:4566',
      cacheTtl: '5m',
      awsCredentials: {
        type: 'access_key',
        accessKeyId: 'test-access-key',
        secretAccessKey: 'test-secret-key',
      },
    });
  });

  it('builds GCP KMS key data from split resource fields', () => {
    const payload = buildKeyDataPayload({
      type: 'gcp_kms',
      fields: {
        gcpProject: 'auth-proxy-446804',
        gcpLocation: 'global',
        gcpKeyRing: 'authproxy',
        gcpCryptoKey: 'authproxy-github-actions-ci-1',
        gcpCredentialsFile: '/tmp/gcp.json',
        cacheTtl: '5m',
      },
    });

    expect(payload).toEqual({
      gcpProject: 'auth-proxy-446804',
      gcpLocation: 'global',
      gcpKeyRing: 'authproxy',
      gcpCryptoKey: 'authproxy-github-actions-ci-1',
      gcpCredentialsFile: '/tmp/gcp.json',
      cacheTtl: '5m',
    });
  });

  it('round-trips API key data into editable state', () => {
    expect(keyDataFormStateFromConfig({
      awsKmsKeyId: 'alias/authproxy',
      awsRegion: 'us-east-1',
      awsCredentials: {
        type: 'access_key',
        accessKeyId: '***************',
        secretAccessKey: '***************',
      },
    })).toEqual({
      type: 'aws_kms',
      fields: {
        numBytes: '32',
        awsCredentialsType: 'access_key',
        awsKmsKeyId: 'alias/authproxy',
        awsRegion: 'us-east-1',
        awsAccessKeyId: '***************',
        awsSecretAccessKey: '***************',
      },
    });
  });

  it('requires redacted secrets to be re-entered before saving key data', () => {
    expect(validateKeyDataFormState({
      type: 'aws_kms',
      fields: {
        awsKmsKeyId: 'alias/authproxy',
        awsCredentialsType: 'access_key',
        awsAccessKeyId: '***************',
        awsSecretAccessKey: '***************',
      },
    })).toBe('AWS Access Key ID must be re-entered to update key data');
  });

  it('validates cloud provider required fields', () => {
    expect(validateKeyDataFormState({
      type: 'gcp_kms',
      fields: {
        gcpProject: 'auth-proxy-446804',
        gcpLocation: 'global',
      },
    })).toBe('GCP Key Ring is required');

    expect(validateKeyDataFormState({
      type: 'aws_kms',
      fields: {
        awsKmsKeyId: 'alias/authproxy',
        awsCredentialsType: 'access_key',
        awsAccessKeyId: 'test-access-key',
      },
    })).toBe('AWS Secret Access Key is required');
  });
});
