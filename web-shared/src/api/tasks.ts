// Task API client
import { api, Paged } from './index';

export interface TaskItem {
  id: string;
  provider: string;
  action: string;
  model: string;
  status: 'pending' | 'running' | 'success' | 'failed';
  progress: number;
  result?: Record<string, unknown>;
  error?: string;
  created_at: string;
  finished_at?: string;
}

export const taskApi = {
  list: (page = 1, size = 20) =>
    api.get<Paged<TaskItem>>(`/api/user/tasks?page=${page}&size=${size}`),
  get: (id: string) => api.get<TaskItem>(`/api/user/tasks/${id}`),
};
