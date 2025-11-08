// ListResponse is a generic response for list endpoints.
export interface ListResponse<T> {
  items: T[];
  cursor?: string;
}
