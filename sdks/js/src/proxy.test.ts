import { AxiosError, AxiosHeaders } from 'axios';
import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import {
  client,
  configureClient,
  type ApiError,
  type ProxyResponse,
  type SendEvidence,
} from './index';

const evidenceValues: SendEvidence[] = ['notSent', 'mayHaveSent', 'sent'];

describe('proxy send evidence', () => {
  beforeEach(() => {
    // Client diagnostics are unrelated to the response contract under test.
    vi.spyOn(console, 'info').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it.each(evidenceValues)('preserves %s evidence in a wrapped response', async (sendEvidence) => {
    const payload: ProxyResponse = { statusCode: 429, headers: {}, bodyRaw: null, bodyJson: null, sendEvidence };
    configureClient({
      axiosConfigOverride: {
        adapter: async (config) => ({
          config, data: payload, status: 200, statusText: 'OK', headers: new AxiosHeaders(),
        }),
      },
    });

    // A provider error status still arrives through the successful outer envelope.
    const response = await client.post<ProxyResponse>('/api/v1/connections/cxn_example/_proxy', {});
    expect(response.status).toBe(200);
    expect(response.data).toEqual(payload);
  });

  it.each(evidenceValues)('preserves %s evidence on an Axios error payload', async (sendEvidence) => {
    const payload: ApiError = { error: 'Proxy failed', sendEvidence };
    configureClient({
      axiosConfigOverride: {
        adapter: async (config) => {
          const response = {
            config, data: payload, status: 502, statusText: 'Bad Gateway', headers: new AxiosHeaders(),
          };
          throw new AxiosError<ApiError>('Proxy failed', AxiosError.ERR_BAD_RESPONSE, config, undefined, response);
        },
      },
    });

    // The shared client exposes metadata on error.response.data without rewriting it.
    await expect(client.post('/api/v1/connections/cxn_example/_proxy', {})).rejects.toMatchObject({
      response: { status: 502, data: payload },
    });
  });

  it('retains missing evidence for older servers and unrelated API errors', async () => {
    const payload: ProxyResponse = { statusCode: 200, headers: {}, bodyJson: { ok: true } };
    const unrelatedError: ApiError = { error: 'Bad Request' };
    configureClient({
      axiosConfigOverride: {
        adapter: async (config) => ({
          config, data: payload, status: 200, statusText: 'OK', headers: new AxiosHeaders(),
        }),
      },
    });

    // Absence is preserved as unknown rather than synthesized as notSent.
    const response = await client.post<ProxyResponse>('/api/v1/connections/cxn_example/_proxy', {});
    expect(response.data.sendEvidence).toBeUndefined();
    expect(unrelatedError).not.toHaveProperty('sendEvidence');
    expectTypeOf<SendEvidence>().toEqualTypeOf<'notSent' | 'mayHaveSent' | 'sent'>();
    expectTypeOf<ProxyResponse['sendEvidence']>().toEqualTypeOf<SendEvidence | undefined>();
    expectTypeOf<ApiError['sendEvidence']>().toEqualTypeOf<SendEvidence | undefined>();
  });
});
