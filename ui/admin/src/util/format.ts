import YAML from 'yaml';

export type Serializable = unknown;

export function formatAsJson(value: Serializable): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    // Fallback best-effort
    return String(value);
  }
}

export function formatAsYaml(value: Serializable): string {
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return YAML.stringify(value as any);
  } catch {
    return String(value);
  }
}
