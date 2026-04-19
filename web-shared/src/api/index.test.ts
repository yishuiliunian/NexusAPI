// api.test.ts —— 验证 api 客户端的核心行为：
//   - credentials: 'include' 始终带上
//   - mutating 方法自动从 document.cookie 读 nexus_csrf 并写入 X-CSRF-Token
//   - 非 2xx 响应抛出 ApiClientError，body 被解包
//   - 204 不解析 json
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiClientError, api, authApi } from './index';

type FetchCall = { input: RequestInfo | URL; init?: RequestInit };

function mockFetch(response: Partial<Response> & { json?: () => Promise<unknown> }) {
  const calls: FetchCall[] = [];
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ input, init });
    return {
      ok: response.ok ?? true,
      status: response.status ?? 200,
      statusText: response.statusText ?? 'OK',
      json: response.json ?? (async () => ({})),
    } as Response;
  });
  vi.stubGlobal('fetch', fn);
  return { calls, fn };
}

beforeEach(() => {
  // 清空 document.cookie
  document.cookie.split(';').forEach((c) => {
    const name = c.split('=')[0]?.trim();
    if (name) document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('api.get', () => {
  it('发送 GET 请求，带 credentials: include', async () => {
    const { calls } = mockFetch({ ok: true, status: 200, json: async () => ({ hello: 'world' }) });
    const out = await api.get<{ hello: string }>('/api/test');
    expect(out).toEqual({ hello: 'world' });
    expect(calls[0].init?.method).toBe('GET');
    expect(calls[0].init?.credentials).toBe('include');
  });

  it('204 不解析 body', async () => {
    mockFetch({ ok: true, status: 204 });
    const out = await api.get<undefined>('/api/empty');
    expect(out).toBeUndefined();
  });

  it('非 2xx 抛 ApiClientError 并解包 body', async () => {
    mockFetch({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      json: async () => ({ code: 'unauthenticated', message: '未登录' }),
    });
    await expect(api.get('/api/fail')).rejects.toBeInstanceOf(ApiClientError);
    try {
      await api.get('/api/fail');
    } catch (e) {
      if (e instanceof ApiClientError) {
        expect(e.status).toBe(401);
        expect(e.body.code).toBe('unauthenticated');
        expect(e.body.message).toBe('未登录');
      }
    }
  });

  it('JSON 解析失败时降级到 statusText', async () => {
    mockFetch({
      ok: false,
      status: 500,
      statusText: 'Internal',
      json: async () => {
        throw new Error('bad json');
      },
    });
    try {
      await api.get('/api/fail');
      expect.fail('应抛异常');
    } catch (e) {
      if (e instanceof ApiClientError) {
        expect(e.body.code).toBe('unknown');
        expect(e.body.message).toBe('Internal');
      }
    }
  });
});

describe('api mutating methods', () => {
  it('POST 从 cookie 读取 nexus_csrf 注入 X-CSRF-Token', async () => {
    document.cookie = 'nexus_csrf=tok-abc; path=/';
    const { calls } = mockFetch({ ok: true, json: async () => ({}) });
    await api.post('/api/do', { a: 1 });
    const init = calls[0].init!;
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ a: 1 }));
    const headers = init.headers as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBe('tok-abc');
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('无 CSRF cookie 时 POST 不报错但不带 header', async () => {
    const { calls } = mockFetch({ ok: true, json: async () => ({}) });
    await api.post('/api/do');
    const headers = calls[0].init!.headers as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBeUndefined();
  });

  it('PUT/DELETE 同样注入 CSRF', async () => {
    document.cookie = 'nexus_csrf=t1; path=/';
    const { calls } = mockFetch({ ok: true, json: async () => ({}) });
    await api.put('/api/x', { v: 2 });
    await api.del('/api/x');
    expect((calls[0].init!.headers as Record<string, string>)['X-CSRF-Token']).toBe('t1');
    expect((calls[1].init!.headers as Record<string, string>)['X-CSRF-Token']).toBe('t1');
    expect(calls[1].init!.method).toBe('DELETE');
  });

  it('GET 不加 CSRF header', async () => {
    document.cookie = 'nexus_csrf=t; path=/';
    const { calls } = mockFetch({ ok: true, json: async () => ({}) });
    await api.get('/api/x');
    const headers = calls[0].init!.headers as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBeUndefined();
  });
});

describe('authApi', () => {
  it('login POST /api/auth/login', async () => {
    const { calls } = mockFetch({
      ok: true,
      json: async () => ({ id: 1, email: 'a@b.com', role: 'user' }),
    });
    await authApi.login('a@b.com', 'pw');
    expect(calls[0].input.toString()).toContain('/api/auth/login');
    expect(calls[0].init?.body).toContain('a@b.com');
  });

  it('logout POST /api/auth/logout', async () => {
    const { calls } = mockFetch({ ok: true, json: async () => ({ ok: true }) });
    await authApi.logout();
    expect(calls[0].init?.method).toBe('POST');
    expect(calls[0].input.toString()).toContain('/api/auth/logout');
  });
});

describe('ApiClientError', () => {
  it('message 来自 body.message', () => {
    const e = new ApiClientError(400, { code: 'invalid', message: '参数错' });
    expect(e.message).toBe('参数错');
    expect(e.status).toBe(400);
    expect(e.body.code).toBe('invalid');
  });
});
