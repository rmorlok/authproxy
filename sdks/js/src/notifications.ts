import {client} from './client';
import {ListResponse} from './common';

export enum NotificationLevel {
    INFO = 'info',
    WARNING = 'warning',
    ERROR = 'error',
}

export enum NotificationState {
    ACTIVE = 'active',
    RESOLVED = 'resolved',
}

export interface Notification {
    id: string;
    key: string;
    level: NotificationLevel;
    state: NotificationState;
    resourceType: string;
    resourceId: string;
    namespace: string;
    title: string;
    message: string;
    actionUrl?: string;
    canAction: boolean;
    viewed: boolean;
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
    resolvedAt?: string;
}

export interface ListNotificationsParams {
    limit?: number;
    includeViewed?: boolean;
    state?: NotificationState;
    namespace?: string;
    labelSelector?: string;
}

export interface MarkNotificationsViewedRequest {
    ids: string[];
}

export const listNotifications = (params?: ListNotificationsParams) => {
    return client.get<ListResponse<Notification>>('/api/v1/notifications', {params});
};

export const markNotificationViewed = (id: string) => {
    return client.post<void>(`/api/v1/notifications/${id}/_viewed`);
};

export const markNotificationsViewed = (ids: string[]) => {
    const request: MarkNotificationsViewedRequest = {ids};
    return client.post<void>('/api/v1/notifications/_viewed', request);
};

export const notifications = {
    list: listNotifications,
    markViewed: markNotificationViewed,
    markBatchViewed: markNotificationsViewed,
};
