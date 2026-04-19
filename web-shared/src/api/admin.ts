// Admin API client — 复用 session cookie，只访问 /api/admin/*。
//
// 字段命名遵循后端 json tag（snake_case），避免 PascalCase/snake 不一致。
import { api, Paged } from './index';

export interface Channel {
  id: number;
  name: string;
  provider: string;
  base_url: string;
  models: string[] | null;
  group_ids: number[] | null;
  weight: number;
  price_multiplier: number;
  status: string;
  latency_ms: number;
  note: string;
  created_at: string;
  updated_at: string;
}

export interface ModelPrice {
  id: number;
  model: string;
  capability: string;
  input_price: number;
  output_price: number;
  cache_price: number;
  output_multiplier: number;
  task_price: number;
  enabled: boolean;
}

export interface AdminUser {
  id: number;
  email: string;
  role: string;
  group_id: number;
  quota: number;
  used_quota: number;
  status: string;
  rpm_limit: number;
  created_at: string;
}

export interface Group {
  id: number;
  name: string;
  price_multiplier: number;
}

export const adminApi = {
  providers: () => api.get<{ providers: string[] }>('/api/admin/providers'),

  listUsers: (page = 1, size = 50) =>
    api.get<Paged<AdminUser>>(`/api/admin/users?page=${page}&size=${size}`),
  createUser: (data: {
    email: string;
    password: string;
    role?: 'user' | 'admin';
    quota?: number;
    group_id?: number;
  }) => api.post<AdminUser>('/api/admin/users', data),
  updateUserQuota: (id: number, quota: number) =>
    api.put(`/api/admin/users/${id}/quota`, { quota }),
  updateUserStatus: (id: number, status: string) =>
    api.put(`/api/admin/users/${id}/status`, { status }),
  updateUserRpm: (id: number, rpmLimit: number) =>
    api.put<{ ok: boolean; rpm_limit: number }>(`/api/admin/users/${id}/rpm-limit`, {
      rpm_limit: rpmLimit,
    }),

  listGroups: () => api.get<Paged<Group>>('/api/admin/groups'),
  createGroup: (name: string, priceMultiplier: number) =>
    api.post<Group>('/api/admin/groups', { name, price_multiplier: priceMultiplier }),
  deleteGroup: (id: number) => api.del(`/api/admin/groups/${id}`),

  listChannels: (page = 1, size = 50) =>
    api.get<Paged<Channel>>(`/api/admin/channels?page=${page}&size=${size}`),
  createChannel: (data: Partial<Channel>) => api.post<Channel>('/api/admin/channels', data),
  updateChannel: (id: number, data: Partial<Channel>) =>
    api.put<Channel>(`/api/admin/channels/${id}`, data),
  deleteChannel: (id: number) => api.del(`/api/admin/channels/${id}`),
  syncChannelModels: (id: number) =>
    api.post<{ models: string[]; count: number }>(`/api/admin/channels/${id}/sync-models`),

  listModels: () => api.get<Paged<ModelPrice>>('/api/admin/models'),
  upsertModel: (data: Partial<ModelPrice>) => api.put<ModelPrice>('/api/admin/models', data),
  deleteModel: (id: number) => api.del(`/api/admin/models/${id}`),
  syncPricing: () =>
    api.post<{ inserted: number; deleted: number; skipped: number }>(
      '/api/admin/models/sync-pricing',
    ),

  listUsages: (page = 1, size = 50) =>
    api.get<Paged<unknown>>(`/api/admin/logs/usages?page=${page}&size=${size}`),

  stats: (days = 7) => api.get<import('./index').AdminStats>(`/api/admin/stats?days=${days}`),
};
