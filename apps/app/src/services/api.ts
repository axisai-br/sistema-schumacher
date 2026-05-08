import { getSupabaseClient } from "./supabase";

const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";
const DEV_TOKEN = import.meta.env.VITE_API_TOKEN ?? "";
const DEBUG_USER_ID = import.meta.env.VITE_DEBUG_USER_ID ?? "";

type RequestOptions = {
  method?: string;
  body?: unknown;
};

type FormRequestOptions = {
  method?: string;
  body: FormData;
};

export class APIRequestError extends Error {
  code?: string;
  details?: unknown;
  requirementsMissing?: string[];

  constructor(message: string, options: { code?: string; details?: unknown; requirementsMissing?: string[] } = {}) {
    super(message);
    this.name = "APIRequestError";
    this.code = options.code;
    this.details = options.details;
    this.requirementsMissing = options.requirementsMissing;
  }
}

async function getAuthHeader() {
  try {
    const client = getSupabaseClient();
    if (client) {
      const { data } = await client.auth.getSession();
      if (data.session?.access_token) {
        return `Bearer ${data.session.access_token}`;
      }
    }
  } catch {
    // ignore and fall back to dev token
  }
  if (DEV_TOKEN) {
    return `Bearer ${DEV_TOKEN}`;
  }
  return "";
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const authHeader = await getAuthHeader();
  const res = await fetch(`${API_URL}${path}`, {
    method: options.method ?? "GET",
    headers: {
      "Content-Type": "application/json",
      ...(authHeader ? { Authorization: authHeader } : {}),
      ...(DEBUG_USER_ID ? { "X-Debug-User-Id": DEBUG_USER_ID } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });

  const text = await res.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { message: text };
    }
  }

  if (!res.ok) {
    const message = data?.message || data?.error || res.statusText;
    throw new APIRequestError(message, {
      code: data?.code,
      details: data?.details,
      requirementsMissing: data?.requirements_missing ?? data?.details?.requirements_missing,
    });
  }

  return data as T;
}

async function requestForm<T>(path: string, options: FormRequestOptions): Promise<T> {
  const authHeader = await getAuthHeader();
  const res = await fetch(`${API_URL}${path}`, {
    method: options.method ?? "POST",
    headers: {
      ...(authHeader ? { Authorization: authHeader } : {}),
      ...(DEBUG_USER_ID ? { "X-Debug-User-Id": DEBUG_USER_ID } : {}),
    },
    body: options.body,
  });

  const text = await res.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { message: text };
    }
  }

  if (!res.ok) {
    const message = data?.message || data?.error || res.statusText;
    throw new APIRequestError(message, {
      code: data?.code,
      details: data?.details,
      requirementsMissing: data?.requirements_missing ?? data?.details?.requirements_missing,
    });
  }

  return data as T;
}

export function apiGet<T>(path: string) {
  return request<T>(path);
}

export function apiPost<T>(path: string, body: unknown) {
  return request<T>(path, { method: "POST", body });
}

export function apiPatch<T>(path: string, body: unknown) {
  return request<T>(path, { method: "PATCH", body });
}

export function apiPut<T>(path: string, body: unknown) {
  return request<T>(path, { method: "PUT", body });
}

export function apiDelete<T>(path: string) {
  return request<T>(path, { method: "DELETE" });
}

export function apiPostForm<T>(path: string, body: FormData) {
  return requestForm<T>(path, { method: "POST", body });
}

export function apiBaseUrl() {
  return API_URL;
}
