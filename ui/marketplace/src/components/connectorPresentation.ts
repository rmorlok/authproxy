import {
  Connector,
  CONNECTOR_KIND,
  VersionedConnectorReference,
  objectReference,
} from '@authproxy/api';

interface ConnectorImage {
  publicUrl?: string;
  base64?: string;
  mimeType?: string;
}

interface SetupFlowPhase {
  steps?: unknown[];
}

interface ConnectorDefinitionPresentation {
  displayName: string;
  description: string;
  highlight: string;
  logoUrl?: string;
  hasConfigure: boolean;
}

const stringValue = (value: unknown): string => (
  typeof value === 'string' ? value : ''
);

const imageUrl = (value: unknown): string | undefined => {
  // Image supports the compact URL form as well as the structured public URL
  // and base64 forms used by connector definitions.
  if (typeof value === 'string') {
    return value;
  }
  if (!value || typeof value !== 'object') {
    return undefined;
  }

  const image = value as ConnectorImage;
  if (image.publicUrl) {
    return image.publicUrl;
  }
  if (image.base64) {
    return `data:${image.mimeType || 'image/png'};base64,${image.base64}`;
  }
  return undefined;
};

export const getConnectorPresentation = (
  connector: Connector,
): ConnectorDefinitionPresentation => {
  const definition = connector.spec.definition;
  const setupFlow = definition.setupFlow && typeof definition.setupFlow === 'object'
    ? definition.setupFlow as { configure?: SetupFlowPhase }
    : undefined;

  return {
    displayName: stringValue(definition.displayName) || connector.metadata.name,
    description: stringValue(definition.description),
    highlight: stringValue(definition.highlight),
    logoUrl: imageUrl(definition.logo),
    hasConfigure: Boolean(setupFlow?.configure?.steps?.length),
  };
};

export const connectorMatchesReference = (
  connector: Connector,
  reference: { id?: string; name?: string; namespace?: string; generation?: number },
): boolean => {
  if (reference.generation !== connector.metadata.generation) {
    return false;
  }
  if (reference.id) {
    return reference.id === connector.metadata.id;
  }
  return reference.name === connector.metadata.name &&
    reference.namespace === connector.metadata.namespace;
};

export const connectorReference = (connector: Connector): VersionedConnectorReference => (
  objectReference(CONNECTOR_KIND, {
    id: connector.metadata.id,
    generation: connector.metadata.generation,
  })
);
