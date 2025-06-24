import { client } from './client';
import { Connector, ListConnectorsResponse } from '../models';

/**
 * Get a list of all available connectors
 * @returns Promise with the list of connectors
 */
export const listConnectors = () => {
  return client.get<ListConnectorsResponse>('/connectors');
};

/**
 * Get a specific connector by ID
 * @param id The ID of the connector to get
 * @returns Promise with the connector details
 */
export const getConnector = (id: string) => {
  return client.get<Connector>(`/connectors/${id}`);
};

export const connectors = {
  list: listConnectors,
  get: getConnector,
};