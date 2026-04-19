'use client';

// API Keys · 管理密钥
//   - 新建（名称）
//   - 列表（前缀 / 后缀 / 状态 / 最后使用）
//   - 删除（确认 dialog）
//   - 一次性 secret 展示

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  ApiClientError,
  ApiKey,
  Badge,
  Button,
  CreateKeyResult,
  EmptyState,
  Input,
  PageContent,
  PageHeader,
  Section,
  userApi,
} from '@nexusapi/shared';
import { UserShell } from '../../components/user-shell';

export default function KeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<CreateKeyResult | null>(null);

  async function load() {
    setLoading(true);
    try {
      const page = await userApi.apiKeys();
      setKeys(page.items ?? []);
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'load failed');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function create() {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const res = await userApi.createKey({ name: newName.trim() });
      setCreated(res);
      setNewName('');
      load();
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'create failed');
    } finally {
      setCreating(false);
    }
  }

  async function del(id: number, name: string) {
    if (!confirm(`确定删除密钥 "${name}"？此操作不可撤销。`)) return;
    try {
      await userApi.deleteKey(id);
      load();
    } catch (e) {
      setErr(e instanceof ApiClientError ? e.body.message : 'delete failed');
    }
  }

  return (
    <UserShell>
      <PageContent>
        <PageHeader
          title="API Keys"
          description="管理访问凭证 · 用于调用 /v1/chat/completions 等中继接口"
        />

        {err && (
          <div className="rounded-md border border-danger bg-danger-bg px-4 py-3 text-sm text-danger">
            {err}
          </div>
        )}

        {created && (
          <div className="rounded-lg border border-success bg-success-bg p-5 shadow-subtle">
            <div className="flex items-center gap-2.5">
              <div className="grid h-5 w-5 place-items-center rounded-full bg-success text-xs text-white">✓</div>
              <h3 className="text-sm font-semibold text-emerald-900">
                密钥已创建（仅展示一次，请立即保存）
              </h3>
            </div>
            <div className="mt-3 flex items-center justify-between gap-3 rounded-md bg-white p-3">
              <code className="font-mono text-xs text-slate-900">{created.secret}</code>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    navigator.clipboard.writeText(created.secret);
                    alert('已复制');
                  }}
                >
                  复制
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setCreated(null)}>
                  关闭
                </Button>
              </div>
            </div>
          </div>
        )}

        <Section
          title="新建密钥"
          description="密钥明文仅展示一次，请妥善保存"
        >
          <div className="flex items-center gap-3 p-5">
            <div className="flex-1">
              <Input
                placeholder="例：production / dev-local / ci-runner"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') create();
                }}
              />
            </div>
            <Button onClick={create} disabled={!newName.trim() || creating}>
              {creating ? '创建中…' : '+ 新建'}
            </Button>
          </div>
        </Section>

        <Section
          title={`全部密钥（${keys.length}）`}
          description="前缀 / 后缀可用于在日志里定位哪个 key 被调用"
        >
          {loading ? (
            <div className="p-5 text-sm text-slate-500">加载中…</div>
          ) : keys.length === 0 ? (
            <EmptyState
              title="还没有密钥"
              description="建一个密钥，开始调用 /v1/* 接口"
            />
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-5 py-3 text-left font-medium">名称</th>
                  <th className="px-5 py-3 text-left font-medium">前缀…后缀</th>
                  <th className="px-5 py-3 text-left font-medium">状态</th>
                  <th className="px-5 py-3 text-left font-medium">创建时间</th>
                  <th className="px-5 py-3 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((k) => (
                  <tr key={k.id} className="border-t border-slate-100">
                    <td className="px-5 py-3 font-medium text-slate-900">{k.name}</td>
                    <td className="px-5 py-3 font-mono text-xs text-slate-500">
                      {k.prefix}...{k.suffix}
                    </td>
                    <td className="px-5 py-3">
                      <Badge variant={k.status === 'active' ? 'success' : 'neutral'} dot>
                        {k.status === 'active' ? 'active' : 'disabled'}
                      </Badge>
                    </td>
                    <td className="px-5 py-3 text-xs text-slate-500">
                      {new Date(k.created_at).toLocaleString('zh-CN')}
                    </td>
                    <td className="px-5 py-3 text-right">
                      <Button variant="ghost" size="sm" onClick={() => del(k.id, k.name)}>
                        删除
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Section>

        <Section title="curl 调用示例">
          <div className="p-5">
            <pre className="overflow-x-auto rounded-md bg-slate-900 p-4 text-xs text-slate-300">
              <code>{`curl -X POST http://127.0.0.1:8080/v1/chat/completions \\
  -H "Authorization: Bearer sk-nexus-..." \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'`}</code>
            </pre>
          </div>
        </Section>

        <div className="text-center">
          <Link href="/dashboard" className="text-xs text-brand-600">
            ← 返回监控看板
          </Link>
        </div>
      </PageContent>
    </UserShell>
  );
}
