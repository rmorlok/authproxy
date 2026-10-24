// @vitest-environment jsdom
import * as React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';
import RequestDetail from './RequestDetail';
import {getRequestEvent, RequestEvent, RequestEventCaptureState} from '@authproxy/api';

vi.mock('@authproxy/api', () => ({
  getRequestEvent: vi.fn(),
}));

// eventWithCaptureState builds a get-endpoint response whose summary loaded
// from the searchable record, with the given blob capture state.
function eventWithCaptureState(captureState: RequestEventCaptureState, message?: string): RequestEvent {
  return {
    apiVersion: 'authproxy.net/v1alpha1',
    kind: 'RequestEvent',
    metadata: {
      id: 'req_test',
      namespace: 'root',
      createdAt: '2026-10-24T00:00:00.000Z',
    },
    spec: {
      requestType: 'proxy',
      durationMilliseconds: 120,
      namespaceRef: {apiVersion: 'authproxy.net/v1alpha1', kind: 'Namespace', id: 'root'},
      request: {method: 'GET', host: 'api.example.com', scheme: 'https', path: '/items'},
      response: {statusCode: 200},
      captureAvailable: true,
    },
    status: {captureState, message},
  } as RequestEvent;
}

function renderDetail() {
  render(
    <MemoryRouter>
      <RequestDetail requestId="req_test" />
    </MemoryRouter>,
  );
}

describe('RequestDetail capture status', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(getRequestEvent).mockReset();
  });

  it('shows the summary with a warning when captured details are unavailable', async () => {
    // The API returns the summary but reports blob storage failed.
    vi.mocked(getRequestEvent).mockResolvedValue({
      status: 200,
      data: eventWithCaptureState('unavailable', 'Captured request details could not be loaded from blob storage.'),
    } as any);

    renderDetail();

    // The warning explains the gap and the summary still renders instead of an error.
    const alert = await screen.findByTestId('request-capture-status');
    expect(alert.textContent).toContain('could not be loaded from blob storage');
    expect(screen.getByRole('tab', {name: 'Overview'})).toBeTruthy();
    expect(screen.queryByText('Failed to load request')).toBeNull();
  });

  it('omits the warning when captured details loaded', async () => {
    // A normal read reports the loaded state.
    vi.mocked(getRequestEvent).mockResolvedValue({
      status: 200,
      data: eventWithCaptureState('loaded'),
    } as any);

    renderDetail();

    // No capture-status alert is shown.
    await screen.findByRole('tab', {name: 'Overview'});
    expect(screen.queryByTestId('request-capture-status')).toBeNull();
  });
});
