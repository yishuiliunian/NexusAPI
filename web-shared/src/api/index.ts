// API client — 封装 fetch，带 credentials: 'include' 自动传递 session cookie。

export interface ApiError {
  code: string;
  message: string;
  request_id?: string;
}

export class ApiClientError extends Error {
  constructor(
    public status: number,
    public body: ApiError
  ) {
    super(body.message);
  }
}

const baseURL = process.env.NEXT_PUBLIC_API_BASE ?? '';

// readCookie 从 document.cookie 中读取指定 cookie 值。
// 仅在浏览器环境调用；SSR 安全。
function readCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const prefix = name + '=';
  for (const part of document.cookie.split(';')) {
    const p = part.trim();
    if (p.startsWith(prefix)) return decodeURIComponent(p.slice(prefix.length));
  }
  return undefined;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  init?: RequestInit
): Promise<T> {
  // 后端 CSRF：mutating method 会校验 X-CSRF-Token == nexus_csrf cookie。
  // 登录成功后服务器已写 cookie；这里仅做 header 镜像。
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> ?? {}),
  };
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = readCookie('nexus_csrf');
    if (csrf) headers['X-CSRF-Token'] = csrf;
  }
  const res = await fetch(baseURL + path, {
    method,
    credentials: 'include',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    ...init,
  });
  if (!res.ok) {
    const err = (await res.json().catch(() => ({ code: 'unknown', message: res.statusText }))) as ApiError;
    throw new ApiClientError(res.status, err);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  get: <T>(path: string, init?: RequestInit) => request<T>('GET', path, undefined, init),
  post: <T>(path: string, body?: unknown, init?: RequestInit) => request<T>('POST', path, body, init),
  put: <T>(path: string, body?: unknown, init?: RequestInit) => request<T>('PUT', path, body, init),
  del: <T>(path: string, init?: RequestInit) => request<T>('DELETE', path, undefined, init),
};

// ---------- 类型定义（对应后端 DTO） ----------

export interface Me {
  id: number;
  email: string;
  email_verified?: boolean;
  role: 'user' | 'admin';
  quota: number;
  used_quota: number;
  quota_alert_at?: number;
  status: 'active' | 'banned';
}

export interface ApiKey {
  id: number;
  name: string;
  prefix: string;
  suffix: string;
  model_whitelist: string[] | null;
  quota_limit: number;
  used_quota: number;
  status: 'active' | 'disabled';
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
}

export interface CreateKeyResult {
  id: number;
  name: string;
  secret: string;
  prefix: string;
  suffix: string;
}

export interface Usage {
  id: number;
  model: string;
  capability: string;
  prompt_tokens: number;
  completion_tokens: number;
  cache_tokens: number;
  cache_write_tokens: number;
  cache_write_1h_tokens: number;
  reasoning_tokens: number;
  cost: number;
  latency_ms: number;
  status: string;
  created_at: string;
}

export interface Paged<T> {
  items: T[];
  total: number;
}

// ---------- 高层 API ----------

export const authApi = {
  register: (email: string, password: string) =>
    api.post<{ id: number; email: string }>('/api/auth/register', { email, password }),
  login: (email: string, password: string) =>
    api.post<Me>('/api/auth/login', { email, password }),
  logout: () => api.post<{ ok: boolean }>('/api/auth/logout'),
};

export const userApi = {
  me: () => api.get<Me>('/api/user/me'),
  apiKeys: () => api.get<Paged<ApiKey>>('/api/user/apikeys'),
  createKey: (data: { name: string; model_whitelist?: string[]; quota_limit?: number }) =>
    api.post<CreateKeyResult>('/api/user/apikeys', data),
  deleteKey: (id: number) => api.del<{ ok: boolean }>(`/api/user/apikeys/${id}`),
  usages: (page = 1, size = 20) =>
    api.get<Paged<Usage>>(`/api/user/usages?page=${page}&size=${size}`),
  stats: (days = 7) => api.get<Stats>(`/api/user/stats?days=${days}`),
};

// ---------- Dashboard 聚合统计 ----------

export interface DailyAgg {
  date: string;
  requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cache_tokens: number;
  cache_write_tokens: number;
  cache_write_1h_tokens: number;
  reasoning_tokens: number;
  cost: number;
}

export interface ModelAgg {
  model: string;
  requests: number;
  cost: number;
}

export interface CapabilityAgg {
  capability: string;
  requests: number;
  cost: number;
}

export interface StatusAgg {
  status: string;
  requests: number;
}

export interface TopUserAgg {
  user_id: number;
  email: string;
  requests: number;
  cost: number;
}

export interface Stats {
  summary: {
    quota: number;
    used_quota: number;
    total_requests: number;
    total_cost: number;
    success_rate: number;
    since: string;
    days: number;
  };
  by_day: DailyAgg[];
  by_model: ModelAgg[];
  by_capability: CapabilityAgg[];
  by_status: StatusAgg[];
}

export interface AdminStats {
  summary: {
    total_requests: number;
    total_cost: number;
    active_users: number;
    success_rate: number;
    since: string;
    days: number;
  };
  by_day: DailyAgg[];
  by_model: ModelAgg[];
  by_capability: CapabilityAgg[];
  by_status: StatusAgg[];
  top_users: TopUserAgg[];
}
