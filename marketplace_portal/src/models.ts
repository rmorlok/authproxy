import { ApiUser } from './api/auth';

// Connector models
export interface ConnectorVersion {
  id: string;
  version: number;
  state: ConnectorVersionState;
  type: string;
  display_name: string;
  description: string;
  logo: string;
}

export interface Connector extends ConnectorVersion {
  versions: number;
  states: ConnectorVersionState[];
}

export enum ConnectorVersionState {
  DRAFT = 'draft',
  PRIMARY = 'primary',
  ACTIVE = 'active',
  ARCHIVED = 'archived'
}

export interface ListConnectorsResponse {
  items: Connector[];
  cursor?: string;
}

// Connection models
export enum ConnectionState {
  CREATED = 'created',
  CONNECTED = 'connected',
  FAILED = 'failed',
  DISCONNECTED = 'disconnected'
}

export interface Connection {
  id: string;
  connector_id: string;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
}

export interface ListConnectionsResponse {
  items: Connection[];
  cursor?: string;
}

// Request models
export interface InitiateConnectionRequest {
  connector_id: string;
  return_to_url: string;
}

export enum InitiateConnectionResponseType {
  REDIRECT = 'redirect'
}

export interface InitiateConnectionResponse {
  id: string;
  type: InitiateConnectionResponseType;
}

export interface InitiateConnectionRedirectResponse extends InitiateConnectionResponse {
  redirect_url: string;
}