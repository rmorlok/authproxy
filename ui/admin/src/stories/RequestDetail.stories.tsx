import type { Meta, StoryObj } from '@storybook/react';
import React, { useState } from 'react';
import Box from '@mui/material/Box';
import type { AxiosAdapter } from 'axios';
import { client, RequestEvent, RequestEventCaptureState } from '@authproxy/api';
import RequestDetail from '../components/RequestDetail';

// Messages mirror the fixed strings the server returns for degraded reads
// (see requestEventStatus in internal/routes/request_events.go).
const CAPTURE_MESSAGES: Record<RequestEventCaptureState, string | undefined> = {
    loaded: undefined,
    missing: 'Captured request details are not stored for this event; only the summary is available.',
    unavailable: 'Captured request details could not be loaded from blob storage; only the summary is available. Try again later.',
};

// requestEventFixture builds a get-endpoint response for a proxied request.
// Captured headers/bodies are included only when the capture loaded.
function requestEventFixture(captureState: RequestEventCaptureState): RequestEvent {
    return {
        apiVersion: 'authproxy.net/v1alpha1',
        kind: 'RequestEvent',
        metadata: {
            id: 'req_storybook0001',
            namespace: 'root.demo',
            createdAt: '2026-10-24T07:37:34.000Z',
        },
        spec: {
            requestType: 'proxy',
            correlationId: 'corr-storybook',
            durationMilliseconds: 182,
            namespaceRef: { apiVersion: 'authproxy.net/v1alpha1', kind: 'Namespace', id: 'root.demo' },
            request: {
                method: 'GET',
                scheme: 'https',
                host: 'api.github.com',
                path: '/user/repos',
                httpVersion: 'HTTP/1.1',
                sizeBytes: 0,
            },
            response: {
                statusCode: 200,
                httpVersion: 'HTTP/2.0',
                sizeBytes: 4821,
                mimeType: 'application/json',
                source: 'upstream',
            },
            captureAvailable: true,
            ...(captureState === 'loaded'
                ? {
                      capture: {
                          request: {
                              url: 'https://api.github.com/user/repos',
                              headers: { Accept: ['application/vnd.github+json'] },
                          },
                          response: {
                              headers: { 'Content-Type': ['application/json'] },
                              body: btoa('[{"id":1,"name":"authproxy"}]'),
                          },
                      },
                  }
                : {}),
        },
        status: { captureState, message: CAPTURE_MESSAGES[captureState] },
    } as RequestEvent;
}

// StubbedRequestDetail points the shared SDK axios client at an in-memory
// adapter that answers the request-event GET, so the real component renders
// each capture state without a backend.
function StubbedRequestDetail({ captureState }: { captureState: RequestEventCaptureState }) {
    useState(() => {
        const adapter: AxiosAdapter = async (config) => ({
            data: requestEventFixture(captureState),
            status: 200,
            statusText: 'OK',
            headers: {},
            config,
        });
        client.defaults.adapter = adapter;
        return null;
    });

    return (
        <Box sx={{ maxWidth: 900, p: 2 }}>
            <RequestDetail key={captureState} requestId="req_storybook0001" />
        </Box>
    );
}

const meta = {
    title: 'Components/RequestDetail',
    component: StubbedRequestDetail,
    parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof StubbedRequestDetail>;

export default meta;
type Story = StoryObj<typeof meta>;

export const CaptureLoaded: Story = { args: { captureState: 'loaded' } };

// Blob storage failed (e.g. S3 503); the summary still renders with a warning.
export const CaptureUnavailable: Story = { args: { captureState: 'unavailable' } };

// No blob exists for the event; the summary renders with an info notice.
export const CaptureMissing: Story = { args: { captureState: 'missing' } };
