import { describe, expect, it } from 'vitest';
import { formatAsJson, formatAsYaml } from './format';

describe('format utils', () => {
  it('formats objects as pretty JSON', () => {
    const obj = { a: 1, b: { c: true } };
    const json = formatAsJson(obj);
    expect(json).toContain('\n');
    expect(json).toContain('  "a": 1');
    expect(json).toContain('  "b": {');
  });

  it('formats objects as YAML', () => {
    const obj = { a: 1, b: { c: true } };
    const y = formatAsYaml(obj);
    expect(y).toContain('a: 1');
    expect(y).toContain('b:');
    expect(y).toContain('c: true');
  });

  it('handles primitive values', () => {
    expect(formatAsJson('x')).toBe('"x"');
    expect(formatAsYaml('x')).toContain('x');
  });
});
