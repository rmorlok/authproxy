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
    resource_type: string;
    resource_id: string;
    namespace: string;
    title: string;
    message: string;
    action_url?: string;
    can_action: boolean;
    viewed: boolean;
    metadata?: Record<string, unknown>;
    created_at: string;
    updated_at: string;
    resolved_at?: string;
}

export interface ListNotificationsParams {
    limit?: number;
    include_viewed?: boolean;
    state?: NotificationState;
    namespace?: string;
    label_selector?: string;
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
