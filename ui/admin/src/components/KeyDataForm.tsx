import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import Typography from '@mui/material/Typography';
import {KeyDataConfig} from '@authproxy/api';

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

const supportedKeySourceTypes = new Set(keySourceTypes.map(opt => opt.value));

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

function keySourceTypeFromConfig(config?: KeyDataConfig): KeySourceType {
  if (!config?.type || !supportedKeySourceTypes.has(config.type as KeySourceType)) {
    return 'random';
  }
  return config.type as KeySourceType;
}

export function createEmptyKeyDataFormState(): KeyDataFormState {
  return {
    type: 'random',
    fields: {
      num_bytes: '32',
      aws_credentials_type: 'implicit',
    },
  };
}

export function keyDataFormStateFromConfig(config?: KeyDataConfig): KeyDataFormState {
  const state = createEmptyKeyDataFormState();
  if (!config) return state;

  return {
    type: keySourceTypeFromConfig(config),
    fields: {
      ...state.fields,
      ...(config.fields || {}),
    },
  };
}

export function buildKeyDataPayload(state: KeyDataFormState): Record<string, unknown> | undefined {
  const fields = state.fields;

  switch (state.type) {
    case 'random': {
      const raw = stringField(fields, 'num_bytes');
      const numBytes = raw ? parseInt(raw, 10) : 32;
      return {
        random: true,
        num_bytes: Number.isFinite(numBytes) && numBytes > 0 ? numBytes : 32,
      };
    }
    case 'value':
      return compact({value: fields.value});
    case 'base64':
      return compact({base64: fields.base64});
    case 'env_var':
      return compact({env_var: fields.env_var});
    case 'env_var_base64':
      return compact({env_var_base64: fields.env_var_base64});
    case 'file':
      return compact({path: fields.path});
    case 'aws_secret':
      return withAwsCredentials(compact({
        aws_secret_id: fields.aws_secret_id,
        aws_region: fields.aws_region,
        aws_secret_key: fields.aws_secret_key,
        cache_ttl: fields.cache_ttl,
      }), fields);
    case 'aws_kms':
      return withAwsCredentials(compact({
        aws_kms_key_id: fields.aws_kms_key_id,
        aws_region: fields.aws_region,
        aws_kms_endpoint: fields.aws_kms_endpoint,
        cache_ttl: fields.cache_ttl,
      }), fields);
    case 'gcp_secret':
      return compact({
        gcp_secret_name: fields.gcp_secret_name,
        gcp_project: fields.gcp_project,
        gcp_secret_version: fields.gcp_secret_version,
        cache_ttl: fields.cache_ttl,
      });
    case 'gcp_kms':
      return compact({
        gcp_kms_key_name: fields.gcp_kms_key_name,
        gcp_project: fields.gcp_project,
        gcp_location: fields.gcp_location,
        gcp_key_ring: fields.gcp_key_ring,
        gcp_crypto_key: fields.gcp_crypto_key,
        gcp_kms_endpoint: fields.gcp_kms_endpoint,
        gcp_credentials_file: fields.gcp_credentials_file,
        gcp_credentials_json: fields.gcp_credentials_json,
        cache_ttl: fields.cache_ttl,
      });
    case 'hashicorp_vault':
      return compact({
        vault_address: fields.vault_address,
        vault_token: fields.vault_token,
        vault_path: fields.vault_path,
        vault_key: fields.vault_key,
        cache_ttl: fields.cache_ttl,
      });
    case 'hashicorp_vault_transit':
      return compact({
        vault_address: fields.vault_address,
        vault_token: fields.vault_token,
        vault_namespace: fields.vault_namespace,
        vault_transit_mount_path: fields.vault_transit_mount_path,
        vault_transit_key_name: fields.vault_transit_key_name,
        cache_ttl: fields.cache_ttl,
      });
    default:
      return undefined;
  }
}

function withAwsCredentials(payload: Record<string, unknown>, fields: Record<string, string>): Record<string, unknown> {
  const credentialsType = stringField(fields, 'aws_credentials_type');
  if (credentialsType === 'access_key') {
    payload.aws_credentials = compact({
      type: 'access_key',
      access_key_id: fields.aws_access_key_id,
      secret_access_key: fields.aws_secret_access_key,
    });
  } else if (credentialsType === 'implicit') {
    payload.aws_credentials = {type: 'implicit'};
  }
  return payload;
}

export function validateKeyDataFormState(state: KeyDataFormState): string | null {
  const fields = state.fields;
  const required = (field: string, label: string): string | null => {
    return stringField(fields, field) ? null : `${label} is required`;
  };
  const awsCredentialsError = (): string | null => {
    if (stringField(fields, 'aws_credentials_type') !== 'access_key') return null;
    return required('aws_access_key_id', 'AWS Access Key ID') ||
      required('aws_secret_access_key', 'AWS Secret Access Key');
  };

  switch (state.type) {
    case 'random': {
      const numBytes = parseInt(stringField(fields, 'num_bytes') || '32', 10);
      return Number.isFinite(numBytes) && numBytes > 0 ? null : 'Number of Bytes must be greater than zero';
    }
    case 'value':
      return required('value', 'Key Value');
    case 'base64':
      return required('base64', 'Base64 Encoded Key');
    case 'env_var':
      return required('env_var', 'Environment Variable Name');
    case 'env_var_base64':
      return required('env_var_base64', 'Base64 Environment Variable Name');
    case 'file':
      return required('path', 'File Path');
    case 'aws_secret':
      return required('aws_secret_id', 'AWS Secret ID') || awsCredentialsError();
    case 'aws_kms':
      return required('aws_kms_key_id', 'AWS KMS Key ID') || awsCredentialsError();
    case 'gcp_secret':
      return required('gcp_secret_name', 'GCP Secret Name');
    case 'gcp_kms': {
      if (stringField(fields, 'gcp_kms_key_name')) return null;
      return required('gcp_project', 'GCP Project') ||
        required('gcp_location', 'GCP Location') ||
        required('gcp_key_ring', 'GCP Key Ring') ||
        required('gcp_crypto_key', 'GCP Crypto Key');
    }
    case 'hashicorp_vault':
      return required('vault_address', 'Vault Address') ||
        required('vault_token', 'Vault Token') ||
        required('vault_path', 'Vault Path') ||
        required('vault_key', 'Vault Key');
    case 'hashicorp_vault_transit':
      return required('vault_address', 'Vault Address') ||
        required('vault_transit_key_name', 'Vault Transit Key Name');
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
        num_bytes: '32',
        aws_credentials_type: 'implicit',
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
          value={fieldValue('num_bytes', '32')}
          onChange={(e) => updateField('num_bytes', e.target.value)}
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
          value={fieldValue('env_var')}
          onChange={(e) => updateField('env_var', e.target.value)}
        />
      );
    case 'env_var_base64':
      return (
        <TextField
          {...common}
          label="Base64 Environment Variable Name"
          value={fieldValue('env_var_base64')}
          onChange={(e) => updateField('env_var_base64', e.target.value)}
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
            value={fieldValue('aws_secret_id')}
            onChange={(e) => updateField('aws_secret_id', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Region"
            value={fieldValue('aws_region')}
            onChange={(e) => updateField('aws_region', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Secret Key"
            value={fieldValue('aws_secret_key')}
            onChange={(e) => updateField('aws_secret_key', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
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
            value={fieldValue('aws_kms_key_id')}
            onChange={(e) => updateField('aws_kms_key_id', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS Region"
            value={fieldValue('aws_region')}
            onChange={(e) => updateField('aws_region', e.target.value)}
          />
          <TextField
            {...common}
            label="AWS KMS Endpoint"
            value={fieldValue('aws_kms_endpoint')}
            onChange={(e) => updateField('aws_kms_endpoint', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
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
            value={fieldValue('gcp_secret_name')}
            onChange={(e) => updateField('gcp_secret_name', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Project"
            value={fieldValue('gcp_project')}
            onChange={(e) => updateField('gcp_project', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Secret Version"
            value={fieldValue('gcp_secret_version')}
            onChange={(e) => updateField('gcp_secret_version', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
          />
        </Stack>
      );
    case 'gcp_kms':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="GCP KMS Key Name"
            value={fieldValue('gcp_kms_key_name')}
            onChange={(e) => updateField('gcp_kms_key_name', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Project"
            value={fieldValue('gcp_project')}
            onChange={(e) => updateField('gcp_project', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Location"
            value={fieldValue('gcp_location')}
            onChange={(e) => updateField('gcp_location', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Key Ring"
            value={fieldValue('gcp_key_ring')}
            onChange={(e) => updateField('gcp_key_ring', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Crypto Key"
            value={fieldValue('gcp_crypto_key')}
            onChange={(e) => updateField('gcp_crypto_key', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP KMS Endpoint"
            value={fieldValue('gcp_kms_endpoint')}
            onChange={(e) => updateField('gcp_kms_endpoint', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Credentials File"
            value={fieldValue('gcp_credentials_file')}
            onChange={(e) => updateField('gcp_credentials_file', e.target.value)}
          />
          <TextField
            {...common}
            label="GCP Credentials JSON"
            value={fieldValue('gcp_credentials_json')}
            onChange={(e) => updateField('gcp_credentials_json', e.target.value)}
            multiline
            minRows={3}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
          />
        </Stack>
      );
    case 'hashicorp_vault':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="Vault Address"
            value={fieldValue('vault_address')}
            onChange={(e) => updateField('vault_address', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Token"
            value={fieldValue('vault_token')}
            onChange={(e) => updateField('vault_token', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Path"
            value={fieldValue('vault_path')}
            onChange={(e) => updateField('vault_path', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Key"
            value={fieldValue('vault_key')}
            onChange={(e) => updateField('vault_key', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
          />
        </Stack>
      );
    case 'hashicorp_vault_transit':
      return (
        <Stack spacing={2}>
          <TextField
            {...common}
            label="Vault Address"
            value={fieldValue('vault_address')}
            onChange={(e) => updateField('vault_address', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Token"
            value={fieldValue('vault_token')}
            onChange={(e) => updateField('vault_token', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Namespace"
            value={fieldValue('vault_namespace')}
            onChange={(e) => updateField('vault_namespace', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Transit Mount Path"
            value={fieldValue('vault_transit_mount_path')}
            onChange={(e) => updateField('vault_transit_mount_path', e.target.value)}
          />
          <TextField
            {...common}
            label="Vault Transit Key Name"
            value={fieldValue('vault_transit_key_name')}
            onChange={(e) => updateField('vault_transit_key_name', e.target.value)}
          />
          <TextField
            {...common}
            label="Cache TTL"
            value={fieldValue('cache_ttl')}
            onChange={(e) => updateField('cache_ttl', e.target.value)}
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
  const credentialsType = value.fields.aws_credentials_type || 'implicit';
  return (
    <Stack spacing={2}>
      <FormControl fullWidth disabled={disabled}>
        <InputLabel id="aws-credentials-type-label">AWS Credentials</InputLabel>
        <Select
          labelId="aws-credentials-type-label"
          value={credentialsType}
          label="AWS Credentials"
          onChange={(e) => updateField('aws_credentials_type', e.target.value)}
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
            value={value.fields.aws_access_key_id || ''}
            onChange={(e) => updateField('aws_access_key_id', e.target.value)}
          />
          <TextField
            fullWidth
            disabled={disabled}
            label="AWS Secret Access Key"
            value={value.fields.aws_secret_access_key || ''}
            onChange={(e) => updateField('aws_secret_access_key', e.target.value)}
          />
        </Stack>
      )}
    </Stack>
  );
}
