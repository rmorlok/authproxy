import { client } from './client';

type ApiSessionInitiateRequest = {
  auth_token?: string;
  return_to_url: string;
};

type ApiSessionInitiateSuccessResponse = {
  actor_id: string;
};

type ApiSessionInitiateFailureResponse = {
  redirect_url: string;
};

type ApiSessionInitiateResponse =
  | ApiSessionInitiateSuccessResponse
  | ApiSessionInitiateFailureResponse;

function isInitiateSessionSuccessResponse(
  response: ApiSessionInitiateResponse
): response is ApiSessionInitiateSuccessResponse {
  return 'actor_id' in response;
}


type ApiSessionTerminateResponse = Record<string, never>;

const initiate = (params: ApiSessionInitiateRequest) => {
  const headers: { Authorization?: string } = {};
  if (params.auth_token) {
    headers.Authorization = `Bearer ${params.auth_token}`;
  }
  return client.post<ApiSessionInitiateResponse>(
    '/api/v1/session/_initiate',
    {
      return_to_url: params.return_to_url,
    },
    {
      headers: headers,
    }
  );
};

const terminate = () => {
  return client.post<ApiSessionTerminateResponse>('/api/v1/session/_terminate');
};

const session = {
  initiate,
  terminate,
};

export { session, isInitiateSessionSuccessResponse };
export type {
  ApiSessionInitiateRequest,
  ApiSessionInitiateSuccessResponse,
  ApiSessionInitiateFailureResponse,
  ApiSessionInitiateResponse,
  ApiSessionTerminateResponse,
};