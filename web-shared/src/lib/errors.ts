// API 错误对象。所有后端非 2xx 响应都解包为此形状。
export interface ApiError {
  code: string;
  message: string;
  requestId?: string;
}
