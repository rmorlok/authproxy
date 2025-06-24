import axios from "axios";

export const client = axios.create({
    baseURL: import.meta.env.VITE_API_BASE_URL,
    timeout: 200,
    withCredentials: true,
    headers: {
        'Accept': 'application/json'
    },
});