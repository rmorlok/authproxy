import React from 'react';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import Typography from '@mui/material/Typography';
import {KeyData} from '@authproxy/api';

export type KeySourceType =
  | 'random'
  | 'value'
  | 'base64'
  | 'env_var'
  | 'env_var_base64'
  | 'file'
  | 'aws_secret'
  | 'aws_kms'
  | 'gcp_secret'
  | 'gcp_kms'
  | 'hashicorp_vault'
  | 'hashicorp_vault_transit';

export type KeyDataFormState = {
  type: KeySourceType;
  fields: Record<string, string>;
};

export const keySourceTypes: { label: string; value: KeySourceType }[] = [
  {label: 'Random', value: 'random'},
  {label: 'Value', value: 'value'},
  {label: 'Base64', value: 'base64'},
  {label: 'Environment Variable', value: 'env_var'},
  {label: 'Base64 Environment Variable', value: 'env_var_base64'},
  {label: 'File', value: 'file'},
  {label: 'AWS Secret', value: 'aws_secret'},
  {label: 'AWS KMS', value: 'aws_kms'},
  {label: 'GCP Secret', value: 'gcp_secret'},
  {label: 'GCP KMS', value: 'gcp_kms'},
  {label: 'HashiCorp Vault', value: 'hashicorp_vault'},
  {label: 'HashiCorp Vault Transit', value: 'hashicorp_vault_transit'},
];

const keySourceTypeLabels = new Map(keySourceTypes.map(opt => [opt.value, opt.label]));

const providerFieldNames = [
  'numBytes',
  'value',
  'base64',
  'envVar',
  'envVarBase64',
  'path',
  'awsSecretId',
  'awsSecretKey',
  'awsRegion',
  'awsKmsKeyId',
  'awsKmsEndpoint',
  'cacheTtl',
  'gcpSecretName',
  'gcpProject',
  'gcpSecretVersion',
  'gcpKmsKeyName',
  'gcpLocation',
  'gcpKeyRing',
  'gcpCryptoKey',
  'gcpKmsEndpoint',
  'gcpCredentialsFile',
  'gcpCredentialsJson',
  'vaultAddress',
  'vaultToken',
  'vaultPath',
  'vaultKey',
  'vaultNamespace',
  'vaultTransitMountPath',
  'vaultTransitKeyName',
];

const displayFieldNames = providerFieldNames.filter(key => key !== 'numBytes');

function stringField(fields: Record<string, string>, key: string): string {
  return fields[key]?.trim() || '';
}

function compact(values: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(values)) {
    if (typeof value === 'string') {
      const trimmed = value.trim();
      if (trimmed !== '') out[key] = trimmed;
      continue;
    }
    if (value != null) out[key] = value;
  }
  return out;
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (value == null || typeof value !== 'object' || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function hasOwn(record: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(record, key);
}

function valueToString(value: unknown): string {
  if (value == null) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);

  const record = asRecord(value);
  if (!record) return '';
  for (const key of ['value', 'base64', 'envVar', 'envVarBase64', 'path']) {
    if (hasOwn(record, key)) return valueToString(record[key]);
  }
  return '';
}

function setStringField(fields: Record<string, string>, key: string, value: unknown) {
  const str = valueToString(value);
  if (str !== '') fields[key] = str;
}

export function isRedactedPlaceholder(value?: string): boolean {
  const trimmed = value?.trim() || '';
  return trimmed.length > 0 && trimmed.split('').every(ch => ch === '*');
}

function secretPayloadValue(value?: string): string | undefined {
  const trimmed = value?.trim() || '';
  if (!trimmed || isRedactedPlaceholder(trimmed)) return undefined;
  return trimmed;
}

function keySourceTypeFromConfig(config?: KeyData): KeySourceType {
  const data = asRecord(config);
  if (!data) return 'random';

  if (hasOwn(data, 'value')) return 'value';
  if (hasOwn(data, 'base64')) return 'base64';
  if (hasOwn(data, 'envVar')) return 'env_var';
  if (hasOwn(data, 'envVarBase64')) return 'env_var_base64';
  if (hasOwn(data, 'path')) return 'file';
  if (hasOwn(data, 'vaultTransitKeyName')) return 'hashicorp_vault_transit';
  if (hasOwn(data, 'vaultAddress')) return 'hashicorp_vault';
  if (hasOwn(data, 'awsKmsKeyId')) return 'aws_kms';
  if (hasOwn(data, 'awsSecretId')) return 'aws_secret';
  if (hasOwn(data, 'gcpKmsKeyName') || hasOwn(data, 'gcpCryptoKey')) return 'gcp_kms';
  if (hasOwn(data, 'gcpSecretName')) return 'gcp_secret';
  return 'random';
}

export function keyDataSourceLabel(config?: KeyData): string {
  const type = keySourceTypeFromConfig(config);
  return keySourceTypeLabels.get(type) || type;
}

export function createEmptyKeyDataFormState(): KeyDataFormState {
  return {
    type: 'random',
    fields: {
      numBytes: '32',
      awsCredentialsType: 'implicit',
    },
  };
}

export function keyDataFormStateFromConfig(config?: KeyData): KeyDataFormState {
  const state = createEmptyKeyDataFormState();
  const data = asRecord(config);
  if (!data) return state;

  const fields = {...state.fields};
  for (const field of providerFieldNames) {
    setStringField(fields, field, data[field]);
  }

  const awsCredentials = asRecord(data.awsCredentials);
  if (awsCredentials) {
    setStringField(fields, 'awsCredentialsType', awsCredentials.type);
    setStringField(fields, 'awsAccessKeyId', awsCredentials.accessKeyId);
    setStringField(fields, 'awsSecretAccessKey', awsCredentials.secretAccessKey);
  }

  return {
    type: keySourceTypeFromConfig(config),
    fields,
  };
}

export function keyDataDisplayFields(config?: KeyData): { key: string; value: string }[] {
  const state = keyDataFormStateFromConfig(config);
  const entries: { key: string; value: string }[] = [];
  for (const field of displayFieldNames) {
    const value = state.fields[field];
    if (!value) continue;
    entries.push({
      key: field,
      value: isRedactedPlaceholder(value) ? 'configured' : value,
    });
  }
  if (state.fields.awsCredentialsType && state.fields.awsCredentialsType !== 'implicit') {
    entries.push({key: 'awsCredentialsType', value: state.fields.awsCredentialsType});
  }
  return entries;
}

export function buildKeyDataPayload(state: KeyDataFormState): Record<string, unknown> | undefined {
  const fields = state.fields;

  switch (state.type) {
    case 'random': {
      const raw = stringField(fields, 'numBytes');
      const numBytes = raw ? parseInt(raw, 10) : 32;
      return {
        random: true,
        numBytes: Number.isFinite(numBytes) && numBytes > 0 ? numBytes : 32,
      };
    }
    case 'value':
      return compact({value: secretPayloadValue(fields.value)});
    case 'base64':
      return compact({base64: secretPayloadValue(fields.base64)});
    case 'env_var':
      return compact({envVar: fields.envVar});
    case 'env_var_base64':
      return compact({envVarBase64: fields.envVarBase64});
    case 'file':
      return compact({path: fields.path});
    case 'aws_secret':
      return withAwsCredentials(compact({
        awsSecretId: fields.awsSecretId,
        awsRegion: fields.awsRegion,
        awsSecretKey: fields.awsSecretKey,
        cacheTtl: fields.cacheTtl,
      }), fields);
    case 'aws_kms':
      return withAwsCredentials(compact({
        awsKmsKeyId: fields.awsKmsKeyId,
        awsRegion: fields.awsRegion,
        awsKmsEndpoint: fields.awsKmsEndpoint,
        cacheTtl: fields.cacheTtl,
      }), fields);
    case 'gcp_secret':
      return compact({
        gcpSecretName: fields.gcpSecretName,
        gcpProject: fields.gcpProject,
        gcpSecretVersion: fields.gcpSecretVersion,
        cacheTtl: fields.cacheTtl,
      });
    case 'gcp_kms':
      return compact({
        gcpKmsKeyName: fields.gcpKmsKeyName,
        gcpProject: fields.gcpProject,
        gcpLocation: fields.gcpLocation,
        gcpKeyRing: fields.gcpKeyRing,
        gcpCryptoKey: fields.gcpCryptoKey,
        gcpKmsEndpoint: fields.gcpKmsEndpoint,
        gcpCredentialsFile: fields.gcpCredentialsFile,
        gcpCredentialsJson: secretPayloadValue(fields.gcpCredentialsJson),
        cacheTtl: fields.cacheTtl,
      });
    case 'hashicorp_vault':
      return compact({
        vaultAddress: fields.vaultAddress,
        vaultToken: secretPayloadValue(fields.vaultToken),
        vaultPath: fields.vaultPath,
        vaultKey: fields.vaultKey,
        cacheTtl: fields.cacheTtl,
      });
    case 'hashicorp_vault_transit':
      return compact({
        vaultAddress: fields.vaultAddress,
        vaultToken: secretPayloadValue(fields.vaultToken),
        vaultNamespace: fields.vaultNamespace,
        vaultTransitMountPath: fields.vaultTransitMountPath,
        vaultTransitKeyName: fields.vaultTransitKeyName,
        cacheTtl: fields.cacheTtl,
      });
    default:
      return undefined;
  }
}

function withAwsCredentials(payload: Record<string, unknown>, fields: Record<string, string>): Record<string, unknown> {
  const credentialsType = stringField(fields, 'awsCredentialsType');
  if (credentialsType === 'access_key') {
    payload.awsCredentials = compact({
      type: 'access_key',
      accessKeyId: secretPayloadValue(fields.awsAccessKeyId),
      secretAccessKey: secretPayloadValue(fields.awsSecretAccessKey),
    });
  } else if (credentialsType === 'implicit') {
    payload.awsCredentials = {type: 'implicit'};
  }
  return payload;
}

export function validateKeyDataFormState(state: KeyDataFormState): string | null {
  const fields = state.fields;
  const required = (field: string, label: string): string | null => {
    return stringField(fields, field) ? null : `${label} is required`;
  };
  const requiredSecret = (field: string, label: string): string | null => {
    if (!stringField(fields, field)) return `${label} is required`;
    return isRedactedPlaceholder(fields[field]) ? `${label} must be re-entered to update key data` : null;
  };
  const optionalSecret = (field: string, label: string): string | null => {
    return isRedactedPlaceholder(fields[field]) ? `${label} must be re-entered or cleared to update key data` : null;
  };
  const awsCredentialsError = (): string | null => {
    if (stringField(fields, 'awsCredentialsType') !== 'access_key') return null;
    return requiredSecret('awsAccessKeyId', 'AWS Access Key ID') ||
      requiredSecret('awsSecretAccessKey', 'AWS Secret Access Key');
  };

  switch (state.type) {
    case 'random': {
      const numBytes = parseInt(stringField(fields, 'numBytes') || '32', 10);
      return Number.isFinite(numBytes) && numBytes > 0 ? null : 'Number of Bytes must be greater than zero';
    }
    case 'value':
      return requiredSecret('value', 'Key Value');
    case 'base64':
      return requiredSecret('base64', 'Base64 Encoded Key');
    case 'env_var':
      return required('envVar', 'Environment Variable Name');
    case 'env_var_base64':
      return required('envVarBase64', 'Base64 Environment Variable Name');
    case 'file':
      return required('path', 'File Path');
    case 'aws_secret':
      return required('awsSecretId', 'AWS Secret ID') || awsCredentialsError();
    case 'aws_kms':
      return required('awsKmsKeyId', 'AWS KMS Key ID') || awsCredentialsError();
    case 'gcp_secret':
      return required('gcpSecretName', 'GCP Secret Name');
    case 'gcp_kms': {
      const credentialsJSONError = optionalSecret('gcpCredentialsJson', 'GCP Credentials JSON');
      if (credentialsJSONError) return credentialsJSONError;
      if (stringField(fields, 'gcpKmsKeyName')) return null;
      return required('gcpProject', 'GCP Project') ||
        required('gcpLocation', 'GCP Location') ||
        required('gcpKeyRing', 'GCP Key Ring') ||
        required('gcpCryptoKey', 'GCP Crypto Key');
    }
    case 'hashicorp_vault':
      return required('vaultAddress', 'Vault Address') ||
        requiredSecret('vaultToken', 'Vault Token') ||
        required('vaultPath', 'Vault Path') ||
        required('vaultKey', 'Vault Key');
    case 'hashicorp_vault_transit': {
      const vaultTokenError = optionalSecret('vaultToken', 'Vault Token');
      if (vaultTokenError) return vaultTokenError;
      return required('vaultAddress', 'Vault Address') ||
        required('vaultTransitKeyName', 'Vault Transit Key Name');
    }
    default:
      return 'Unsupported key source';
  }
}

export default function KeyDataForm({
  value,
  onChange,
  disabled,
}: {
  value: KeyDataFormState;
  onChange: (value: KeyDataFormState) => void;
  disabled?: boolean;
}) {
  const updateType = (type: KeySourceType) => {
    onChange({
      type,
      fields: {
        numBytes: '32',
        awsCredentialsType: 'implicit',
      },
    });
  };
  const updateField = (field: string, fieldValue: string) => {
    onChange({
      ...value,
      fields: {
        ...value.fields,
        [field]: fieldValue,
      },
    });
  };

  return (
    <Stack spacing={2}>
      <Typography variant="subtitle2" color="text.secondary">Key Data</Typography>
      <FormControl fullWidth disabled={disabled}>
        <InputLabel id="key-source-type-label">Key Source</InputLabel>
        <Select
          labelId="key-source-type-label"
          value={value.type}
          label="Key Source"
          onChange={(e) => updateType(e.target.value as KeySourceType)}
        >
          {keySourceTypes.map(opt => (
            <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>
          ))}
        </Select>
      </FormControl>
      {renderKeySourceFields(value, updateField, disabled)}
    </Stack>
  );
}

function renderKeySourceFields(
  value: KeyDataFormState,
  updateField: (field: string, value: string) => void,
  disabled?: boolean,
) {
  const fieldValue = (field: string, fallback = '') => value.fields[field] ?? fallback;
  const common = {
    fullWidth: true,
    disabled,
  };

  switch (value.type) {
    case 'random':
      return (
        <TextField
          {...common}
          label="Number of Bytes"
          type="number"
          value={fieldValue('numBytes', '32')}
          onChange={(e) => updateField('numBytes', e.target.value)}
        />
      );
    case 'value':
      return (
        <TextField
          {...common}
          label="Key Value"
          value={fieldValue('value')}
          onChange={(e) => updateField('value', e.target.value)}
        />
      );
    case 'base64':
      return (
        <TextField
          {...common}
          label="Base64 Encoded Key"
          value={fieldValue('base64')}
          onChange={(e) => updateField('base64', e.target.value)}
        />
      );
    case 'env_var':
      return (
        <TextField
          {...common}
          label="Environment Variable Name"
          value={fieldValue('envVar')}
          onChange={(e) => updateField('envVar', e.target.value)}
        />
      );
    case 'env_var_base64':
      return (
        <TextField
          {...common}
          label="Base64 Environment Variable Name"
          value={fieldValue('envVarBase64')}
          onChange={(e) => updateField('envVarBase64', e.target.value)}
        />
      );
    case 'file':
      return (
        <TextField
          {...common}
          label="File Path"
          value={fieldValue('path')}
          onChange={(e) => updateField('path', e.target.value)}
        />
      );
    case 'aws_secret':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="AWS Secret ID"
            value={fieldValue('awsSecretId')}
            onChange={(e) => updateField('awsSecretId', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Region"
            value={fieldValue('awsRegion')}
            onChange={(e) => updateField('awsRegion', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Secret Key"
            value={fieldValue('awsSecretKey')}
            onChange={(e) => updateField('awsSecretKey', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
          {renderAwsCredentials(value, updateField, disabled)}
        </Stack>
      );
    case 'aws_kms':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="AWS KMS Key ID"
            value={fieldValue('awsKmsKeyId')}
            onChange={(e) => updateField('awsKmsKeyId', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Region"
            value={fieldValue('awsRegion')}
            onChange={(e) => updateField('awsRegion', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS KMS Endpoint"
            value={fieldValue('awsKmsEndpoint')}
            onChange={(e) => updateField('awsKmsEndpoint', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
          {renderAwsCredentials(value, updateField, disabled)}
        </Stack>
      );
    case 'gcp_secret':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="GCP Secret Name"
            value={fieldValue('gcpSecretName')}
            onChange={(e) => updateField('gcpSecretName', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Project"
            value={fieldValue('gcpProject')}
            onChange={(e) => updateField('gcpProject', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Secret Version"
            value={fieldValue('gcpSecretVersion')}
            onChange={(e) => updateField('gcpSecretVersion', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
        </Stack>
      );
    case 'gcp_kms':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="GCP KMS Key Name"
            value={fieldValue('gcpKmsKeyName')}
            onChange={(e) => updateField('gcpKmsKeyName', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Project"
            value={fieldValue('gcpProject')}
            onChange={(e) => updateField('gcpProject', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Location"
            value={fieldValue('gcpLocation')}
            onChange={(e) => updateField('gcpLocation', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Key Ring"
            value={fieldValue('gcpKeyRing')}
            onChange={(e) => updateField('gcpKeyRing', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Crypto Key"
            value={fieldValue('gcpCryptoKey')}
            onChange={(e) => updateField('gcpCryptoKey', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP KMS Endpoint"
            value={fieldValue('gcpKmsEndpoint')}
            onChange={(e) => updateField('gcpKmsEndpoint', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Credentials File"
            value={fieldValue('gcpCredentialsFile')}
            onChange={(e) => updateField('gcpCredentialsFile', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Credentials JSON"
            value={fieldValue('gcpCredentialsJson')}
            onChange={(e) => updateField('gcpCredentialsJson', e.target.value)}
            multiline
            minRows={3}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
        </Stack>
      );
    case 'hashicorp_vault':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="Vault Address"
            value={fieldValue('vaultAddress')}
            onChange={(e) => updateField('vaultAddress', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Token"
            value={fieldValue('vaultToken')}
            onChange={(e) => updateField('vaultToken', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Path"
            value={fieldValue('vaultPath')}
            onChange={(e) => updateField('vaultPath', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Key"
            value={fieldValue('vaultKey')}
            onChange={(e) => updateField('vaultKey', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
        </Stack>
      );
    case 'hashicorp_vault_transit':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="Vault Address"
            value={fieldValue('vaultAddress')}
            onChange={(e) => updateField('vaultAddress', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Token"
            value={fieldValue('vaultToken')}
            onChange={(e) => updateField('vaultToken', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Namespace"
            value={fieldValue('vaultNamespace')}
            onChange={(e) => updateField('vaultNamespace', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Transit Mount Path"
            value={fieldValue('vaultTransitMountPath')}
            onChange={(e) => updateField('vaultTransitMountPath', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Transit Key Name"
            value={fieldValue('vaultTransitKeyName')}
            onChange={(e) => updateField('vaultTransitKeyName', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cacheTtl')}
            onChange={(e) => updateField('cacheTtl', e.target.value)}
          />
        </Stack>
      );
    default:
      return null;
  }
}

function renderAwsCredentials(
  value: KeyDataFormState,
  updateField: (field: string, value: string) => void,
  disabled?: boolean,
) {
  const credentialsType = value.fields.awsCredentialsType || 'implicit';
  return (
    <Stack spacing={2}>
      <FormControl fullWidth disabled={disabled}>
        <InputLabel id="aws-credentials-type-label">AWS Credentials</InputLabel>
        <Select
          labelId="aws-credentials-type-label"
          value={credentialsType}
          label="AWS Credentials"
          onChange={(e) => updateField('awsCredentialsType', e.target.value)}
        >
          <MenuItem value="">Default</MenuItem>
          <MenuItem value="implicit">Implicit</MenuItem>
          <MenuItem value="access_key">Access Key</MenuItem>
        </Select>
      </FormControl>
      {credentialsType === 'access_key' && (
        <Stack spacing={2}>
          <TextField
            fullWidth
            disabled={disabled}
            label="AWS Access Key ID"
            value={value.fields.awsAccessKeyId || ''}
            onChange={(e) => updateField('awsAccessKeyId', e.target.value)}
          />
          <TextField
            fullWidth
            disabled={disabled}
            label="AWS Secret Access Key"
            value={value.fields.awsSecretAccessKey || ''}
            onChange={(e) => updateField('awsSecretAccessKey', e.target.value)}
          />
        </Stack>
      )}
    </Stack>
  );
}
