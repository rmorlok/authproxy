import { client } from './client';
import {
  API_VERSION,
  ActionRequest,
  ActionResponse,
  ManagedResourceKind,
  ObjectMetadata,
  ObjectReference,
  ResourceList,
  TypeMeta,
  actionRequest,
  objectReference,
} from './common';

export const NOTIFICATION_KIND = 'Notification' as const;
export const NOTIFICATION_VIEW_KIND = 'NotificationView' as const;
export const NOTIFICATION_BATCH_VIEW_KIND = 'NotificationBatchView' as const;

export enum NotificationLevel {
  INFO = 'info',
  WARNING = 'warning',
  ERROR = 'error',
}

export enum NotificationState {
  ACTIVE = 'active',
  RESOLVED = 'resolved',
}

export interface NotificationMetadata extends ObjectMetadata {
  id: string;
  namespace: string;
  createdAt: string;
  updatedAt: string;
}

export interface NotificationSpec {
  key: string;
  level: NotificationLevel;
  resourceRef: ObjectReference<ManagedResourceKind>;
  title: string;
  message: string;
  context?: Record<string, unknown>;
}

export interface NotificationStatus {
  state: NotificationState;
  viewed: boolean;
  action?: {
    url: string;
  };
  resolvedAt?: string;
}

export interface Notification extends TypeMeta<typeof NOTIFICATION_KIND> {
  metadata: NotificationMetadata;
  spec: NotificationSpec;
  status: NotificationStatus;
}

export type NotificationList = ResourceList<Notification>;

export type NotificationReference = ObjectReference<typeof NOTIFICATION_KIND> & {
  id: string;
  name?: never;
  namespace?: never;
  generation?: never;
};

export type NotificationViewRequest = ActionRequest<
  typeof NOTIFICATION_VIEW_KIND,
  typeof NOTIFICATION_KIND,
  Record<string, never>
>;

export type NotificationViewResponse = ActionResponse<
  typeof NOTIFICATION_VIEW_KIND,
  typeof NOTIFICATION_KIND,
  Record<string, never>,
  { viewed: true }
>;

export interface NotificationBatchViewRequest extends TypeMeta<typeof NOTIFICATION_BATCH_VIEW_KIND> {
  metadata: {
    targets: NotificationReference[];
  };
  spec: Record<string, never>;
  status?: never;
}

export interface NotificationBatchViewResponse
  extends TypeMeta<typeof NOTIFICATION_BATCH_VIEW_KIND> {
  metadata: {
    targets: NotificationReference[];
  };
  spec: Record<string, never>;
  status: {
    viewedCount: number;
  };
}

export interface ListNotificationsParams {
  limit?: number;
  includeViewed?: boolean;
  state?: NotificationState;
  namespace?: string;
  labelSelector?: string;
}

const notificationReference = (id: string): NotificationReference =>
  objectReference(NOTIFICATION_KIND, { id }) as NotificationReference;

export const listNotifications = (params?: ListNotificationsParams) =>
  client.get<NotificationList>('/api/v1/notifications', { params });

export const markNotificationViewed = (id: string) => {
  const request: NotificationViewRequest = actionRequest(
    NOTIFICATION_VIEW_KIND,
    notificationReference(id),
    {},
  );
  return client.post<NotificationViewResponse>(`/api/v1/notifications/${id}/_viewed`, request);
};

export const markNotificationsViewed = (ids: string[]) => {
  const request: NotificationBatchViewRequest = {
    apiVersion: API_VERSION,
    kind: NOTIFICATION_BATCH_VIEW_KIND,
    metadata: { targets: ids.map(notificationReference) },
    spec: {},
  };
  return client.post<NotificationBatchViewResponse>('/api/v1/notifications/_viewed', request);
};

export const notifications = {
  list: listNotifications,
  markViewed: markNotificationViewed,
  markBatchViewed: markNotificationsViewed,
};
