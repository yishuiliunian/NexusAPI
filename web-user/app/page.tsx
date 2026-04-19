import Link from 'next/link';

export default function HomePage() {
  return (
    <main className="grid min-h-screen place-items-center bg-gradient-to-br from-brand-50 to-white p-8">
      <div className="max-w-2xl space-y-6 text-center">
        <div className="inline-grid h-14 w-14 place-items-center rounded-xl bg-brand-600 text-2xl font-bold text-white shadow-lifted">
          N
        </div>
        <h1 className="text-5xl font-bold tracking-tight text-slate-900">NexusAPI</h1>
        <p className="text-xl text-slate-700">AI 大模型中转网关</p>
        <p className="text-sm text-slate-500">
          对接 OpenAI / Claude / Gemini 等 40+ 供应商，按量计费，OpenAI 兼容 API。
        </p>
        <div className="flex justify-center gap-3 pt-4">
          <Link
            href="/login"
            className="rounded-md bg-brand-600 px-6 py-2.5 text-sm font-medium text-white shadow-subtle hover:bg-brand-500"
          >
            登录
          </Link>
          <Link
            href="/register"
            className="rounded-md border border-slate-300 bg-white px-6 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            注册
          </Link>
        </div>
      </div>
    </main>
  );
}
