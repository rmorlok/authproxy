import { client } from './client';

type ApiSessionInitiateRequest = {
  authToken?: string;
  returnToUrl: string;
};

type ApiSessionInitiateSuccessResponse = {
  actorId: string;
};

type ApiSessionInitiateFailureResponse = {
  redirectUrl: string;
};

type ApiSessionInitiateResponse =
  | ApiSessionInitiateSuccessResponse
  | ApiSessionInitiateFailureResponse;

function isInitiateSessionSuccessResponse(
  response: ApiSessionInitiateResponse
): response is ApiSessionInitiateSuccessResponse {
  return 'actorId' in response;
}


type ApiSessionTerminateResponse = Record<string, never>;

const initiate = (params: ApiSessionInitiateRequest) => {
  const headers: { Authorization?: string } = {};
  if (params.authToken) {
    headers.Authorization = `Bearer ${params.authToken}`;
  }
  return client.post<ApiSessionInitiateResponse>(
    '/api/v1/session/_initiate',
    {
      returnToUrl: params.returnToUrl,
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
