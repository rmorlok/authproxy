import { describe, expect, test } from 'vitest';
import {
  connectorMatchesReference,
  connectorReference,
  getConnectorPresentation,
} from '../components/connectorPresentation';
import { connectorFixture } from '../testing/resources';

describe('connector presentation', () => {
  test('projects marketplace fields from spec.definition', () => {
    const connector = connectorFixture({
      displayName: 'Example Provider',
      description: 'A detailed description.',
      highlight: 'A short description.',
      logo: {mimeType: 'image/svg+xml', base64: 'PHN2Zy8+'},
      hasConfigure: true,
    });

    expect(getConnectorPresentation(connector)).toEqual({
      displayName: 'Example Provider',
      description: 'A detailed description.',
      highlight: 'A short description.',
      logoUrl: 'data:image/svg+xml;base64,PHN2Zy8+',
      hasConfigure: true,
    });
  });

  test('requires connection references to match the connector generation', () => {
    const connector = connectorFixture({id: 'provider', generation: 3});

    expect(connectorMatchesReference(connector, {id: 'provider', generation: 3})).toBe(true);
    expect(connectorMatchesReference(connector, {id: 'provider', generation: 2})).toBe(false);
    expect(connectorReference(connector)).toMatchObject({
      apiVersion: 'authproxy.net/v1alpha1',
      kind: 'Connector',
      id: 'provider',
      generation: 3,
    });
  });
});
