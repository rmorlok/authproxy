import {client} from "./client";

type ApiError = {
    error: string
};

type ApiListProvidersResponse = string[]

type ApiLogoutResponse = {}

type ApiUser = {
    name: string;
    id: string;
    picture: string;
    aud?: string;
    ip?: string;
    email?: string;
    attrs?: [string: any];
    role?: string;
}

const listProviders = () => {
    return client.get<ApiListProvidersResponse>('/auth/providers');
}

const logout = () => {
    return client.get<ApiLogoutResponse>('/auth/logout');
}

const getUser = () => {
    return client.get<ApiUser>('/auth/user');
}

// This is the same as the core auth user call, but for debugging purposes
const getMe = () => {
    return client.get<ApiUser>('/api/me');
}

const auth = {
    listProviders,
    logout,
    getUser,
    getMe,
};

export { auth };
export type {
    ApiError,
    ApiListProvidersResponse,
    ApiLogoutResponse,
    ApiUser,
};