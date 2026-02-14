// ListResponse is a generic response for list endpoints.
export interface ListResponse<T> {
  items: T[];
  cursor?: string;
  total?: number; // Total is not returned for all endpoints and should not be assumed.
}
