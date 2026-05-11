// upstream-mock.mjs —— 假装是 OpenAI / Anthropic / GitHub OAuth / Google OAuth 上游。
//
// 路径：
//   POST /chat/completions     → OpenAI chat（非流 / 流式）
//   POST /v1/chat/completions  → 同上
//   POST /messages             → Anthropic messages
//   POST /v1/messages          → 同上
//
//   GET  /oauth/github/authorize  → 302 回跳到 redirect_uri（带 code+state）
//   POST /oauth/github/token      → 发 access_token
//   GET  /user                    → GitHub user 信息（需 Bearer）
//   GET  /user/emails             → GitHub emails fallback
//
//   GET  /oauth/google/authorize  → 302 回跳
//   POST /oauth/google/token      → access_token
//   GET  /oauth/google/userinfo   → google userinfo
//
// 由 global-setup.ts 启动，端口默认 18090（可通过 UPSTREAM_PORT 覆盖）。
import { createServer } from 'node:http';

const PORT = Number(process.env.UPSTREAM_PORT ?? 18090);

function send(res, status, body, headers = {}) {
  res.writeHead(status, { 'Content-Type': 'application/json', ...headers });
  res.end(typeof body === 'string' ? body : JSON.stringify(body));
}

function sendSSE(res, events) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  for (const ev of events) {
    if (ev.event) res.write(`event: ${ev.event}\n`);
    res.write(`data: ${typeof ev.data === 'string' ? ev.data : JSON.stringify(ev.data)}\n\n`);
  }
  res.end();
}

function parseBody(req) {
  return new Promise((resolve, reject) => {
    let data = '';
    req.on('data', (c) => (data += c));
    req.on('end', () => {
      try {
        resolve(data ? JSON.parse(data) : {});
      } catch {
        // 尝试 form-encoded
        const obj = Object.fromEntries(new URLSearchParams(data));
        resolve(obj);
      }
    });
    req.on('error', reject);
  });
}

// OAuth mock：in-memory 存 code → user 映射。
// E2E 从 authorize 302 到 redirect_uri?code=xxx&state=yyy；
// backend 拿 code 调 /token 得到 access_token；再调 /user 得 profile。
const oauthCodes = new Map(); // code → { provider, userPayload }

function genCode() {
  return `mock-code-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

const server = createServer(async (req, res) => {
  const url = req.url ?? '/';
  const [pathOnly, query = ''] = url.split('?');
  const q = Object.fromEntries(new URLSearchParams(query));

  if (req.method === 'GET' && pathOnly === '/healthz') {
    return send(res, 200, { ok: true });
  }

  // ---------- /v1/models —— OpenAI 兼容 + Anthropic 共用该端点 ----------
  if (req.method === 'GET' && (pathOnly === '/v1/models' || pathOnly === '/models')) {
    // 无论 OpenAI（Authorization: Bearer）还是 Anthropic（x-api-key）都返回相同 mock 列表。
    // 关键：保留 seed 出来的 e2e 测试用模型（embedding / claude-3-5-sonnet），
    // 避免 admin sync-models 把 seed 的 mock-openai.models 冲掉导致后续 spec 路由失败。
    return send(res, 200, {
      data: [
        { id: 'gpt-4o', object: 'model' },
        { id: 'gpt-4o-mini', object: 'model' },
        { id: 'claude-3-5-sonnet', object: 'model' },
        { id: 'claude-sonnet-4-5', object: 'model' },
        { id: 'text-embedding-3-small', object: 'model' },
      ],
    });
  }
  // Gemini: /v1beta/models?key=...
  if (req.method === 'GET' && pathOnly === '/v1beta/models') {
    return send(res, 200, {
      models: [
        { name: 'models/gemini-1.5-pro' },
        { name: 'models/gemini-2.0-flash' },
      ],
    });
  }

  // ---------- OAuth (GitHub) ----------
  if (req.method === 'GET' && pathOnly === '/oauth/github/authorize') {
    // authorize 302 带 code + state 回到 redirect_uri
    const redirect = q.redirect_uri;
    const state = q.state ?? '';
    if (!redirect) return send(res, 400, { error: 'missing redirect_uri' });
    const code = genCode();
    // 可通过 q.login_email 指定"假用户"邮箱；默认随机
    const email = q.login_email ?? `gh-${Math.random().toString(36).slice(2, 8)}@e2e.test`;
    oauthCodes.set(code, {
      provider: 'github',
      user: {
        id: Math.floor(Math.random() * 1_000_000) + 1,
        login: email.split('@')[0],
        name: email.split('@')[0],
        email,
      },
    });
    const loc = `${redirect}?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`;
    res.writeHead(302, { Location: loc });
    res.end();
    return;
  }
  if (req.method === 'POST' && pathOnly === '/oauth/github/token') {
    const body = await parseBody(req);
    const rec = oauthCodes.get(body.code);
    if (!rec) return send(res, 400, { error: 'bad_verification_code' });
    return send(res, 200, { access_token: `gh-token-${body.code}`, token_type: 'bearer', scope: 'read:user user:email' });
  }
  if (req.method === 'GET' && pathOnly === '/user') {
    // GitHub user info 端点：Authorization: Bearer gh-token-<code>
    const auth = req.headers.authorization ?? '';
    const m = /Bearer gh-token-(.+)/.exec(auth);
    if (!m) return send(res, 401, { message: 'no auth' });
    const rec = oauthCodes.get(m[1]);
    if (!rec) return send(res, 401, { message: 'token unknown' });
    return send(res, 200, rec.user);
  }
  if (req.method === 'GET' && pathOnly === '/user/emails') {
    const auth = req.headers.authorization ?? '';
    const m = /Bearer gh-token-(.+)/.exec(auth);
    if (!m) return send(res, 401, { message: 'no auth' });
    const rec = oauthCodes.get(m[1]);
    if (!rec) return send(res, 401, { message: 'token unknown' });
    return send(res, 200, [{ email: rec.user.email, primary: true, verified: true }]);
  }

  // ---------- OAuth (Google) ----------
  if (req.method === 'GET' && pathOnly === '/oauth/google/authorize') {
    const redirect = q.redirect_uri;
    const state = q.state ?? '';
    if (!redirect) return send(res, 400, { error: 'missing redirect_uri' });
    const code = genCode();
    const email = q.login_email ?? `gg-${Math.random().toString(36).slice(2, 8)}@e2e.test`;
    oauthCodes.set(code, {
      provider: 'google',
      user: {
        sub: `gg-${Date.now()}`,
        email,
        name: email.split('@')[0],
        email_verified: true,
      },
    });
    const loc = `${redirect}?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`;
    res.writeHead(302, { Location: loc });
    res.end();
    return;
  }
  if (req.method === 'POST' && pathOnly === '/oauth/google/token') {
    const body = await parseBody(req);
    const rec = oauthCodes.get(body.code);
    if (!rec) return send(res, 400, { error: 'invalid_grant' });
    return send(res, 200, {
      access_token: `gg-token-${body.code}`,
      id_token: 'fake.jwt.token',
      token_type: 'Bearer',
      expires_in: 3600,
    });
  }
  if (req.method === 'GET' && pathOnly === '/oauth/google/userinfo') {
    const auth = req.headers.authorization ?? '';
    const m = /Bearer gg-token-(.+)/.exec(auth);
    if (!m) return send(res, 401, { error: 'no auth' });
    const rec = oauthCodes.get(m[1]);
    if (!rec) return send(res, 401, { error: 'token unknown' });
    return send(res, 200, rec.user);
  }

  // ---------- Stripe Checkout Session mock ----------
  if (req.method === 'POST' && (pathOnly === '/v1/checkout/sessions' || pathOnly.endsWith('/checkout/sessions'))) {
    // 接收 form-encoded；返回 JSON 包含 id + url
    const id = `cs_test_${Date.now()}`;
    return send(res, 200, {
      id,
      url: `http://127.0.0.1:13000/billing?cs=${id}`,
      status: 'open',
      payment_status: 'unpaid',
    });
  }

  // ---------- AI 任务 mock (midjourney / suno) ----------
  // GET /mj/task/:id/fetch
  {
    const mj = /^\/mj\/task\/([^/]+)\/fetch$/.exec(pathOnly);
    if (req.method === 'GET' && mj) {
      return send(res, 200, {
        id: mj[1],
        status: 'SUCCESS',
        progress: '100%',
        imageUrl: `http://127.0.0.1:${PORT}/mock/${mj[1]}.png`,
      });
    }
  }
  // GET /suno/fetch/:id
  {
    const suno = /^\/suno\/fetch\/([^/]+)$/.exec(pathOnly);
    if (req.method === 'GET' && suno) {
      return send(res, 200, {
        status: 'success',
        audio_url: `http://127.0.0.1:${PORT}/mock/${suno[1]}.mp3`,
        task_id: suno[1],
      });
    }
  }

  // ---------- OpenAI / Anthropic passthrough mock ----------
  if (req.method !== 'POST') {
    return send(res, 404, { error: `not found ${pathOnly}` });
  }

  let body;
  try {
    body = await parseBody(req);
  } catch {
    return send(res, 400, { error: 'bad json' });
  }
  const streaming = body.stream === true;

  // POST /mj/submit/{action} - midjourney
  if (/^\/mj\/submit\//.test(pathOnly)) {
    return send(res, 200, { code: 1, result: `mj-${Date.now()}` });
  }
  // POST /suno/submit/{action}
  if (/^\/suno\/submit\//.test(pathOnly)) {
    return send(res, 200, { task_id: `suno-${Date.now()}` });
  }

  // POST /v1/embeddings (OpenAI 协议)
  if (pathOnly.endsWith('/embeddings')) {
    return send(res, 200, {
      object: 'list',
      model: body.model ?? 'text-embedding-3-small',
      data: [{ object: 'embedding', index: 0, embedding: [0.1, 0.2, 0.3] }],
      usage: { prompt_tokens: 5, total_tokens: 5 },
    });
  }

  if (pathOnly.endsWith('/chat/completions')) {
    if (streaming) {
      return sendSSE(res, [
        { data: { id: 'chatcmpl-1', choices: [{ delta: { content: 'Hello' } }] } },
        { data: { id: 'chatcmpl-1', choices: [{ delta: { content: ' world' } }] } },
        {
          data: {
            id: 'chatcmpl-1',
            choices: [{ delta: {}, finish_reason: 'stop' }],
            usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
          },
        },
        { data: '[DONE]' },
      ]);
    }
    return send(res, 200, {
      id: 'chatcmpl-1',
      object: 'chat.completion',
      model: body.model ?? 'gpt-4o-mini',
      choices: [
        {
          index: 0,
          message: { role: 'assistant', content: 'Hello world' },
          finish_reason: 'stop',
        },
      ],
      usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
    });
  }

  if (pathOnly.endsWith('/messages')) {
    if (streaming) {
      return sendSSE(res, [
        { event: 'message_start', data: { type: 'message_start', message: { id: 'msg_1', usage: { input_tokens: 10 } } } },
        { event: 'content_block_start', data: { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } } },
        { event: 'content_block_delta', data: { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'Hello' } } },
        { event: 'content_block_delta', data: { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: ' world' } } },
        { event: 'message_delta', data: { type: 'message_delta', usage: { output_tokens: 5 } } },
        { event: 'message_stop', data: { type: 'message_stop' } },
      ]);
    }
    return send(res, 200, {
      id: 'msg_1',
      type: 'message',
      role: 'assistant',
      content: [{ type: 'text', text: 'Hello world' }],
      model: body.model ?? 'claude-3-5-sonnet',
      usage: { input_tokens: 10, output_tokens: 5 },
    });
  }

  send(res, 404, { error: `no mock for ${pathOnly}` });
});

// 监听 host：容器内必须 0.0.0.0 以便从宿主机/其他容器访问；
// 直接 spawn 在 host 时绑 127.0.0.1 更安全。用 UPSTREAM_HOST 显式控制。
const HOST = process.env.UPSTREAM_HOST ?? '127.0.0.1';
server.listen(PORT, HOST, () => {
  console.log(`[upstream-mock] listening on http://${HOST}:${PORT}`);
});

process.on('SIGTERM', () => server.close(() => process.exit(0)));
process.on('SIGINT', () => server.close(() => process.exit(0)));
