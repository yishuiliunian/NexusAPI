import Link from 'next/link';

export default function AdminHome() {
  return (
    <main className="grid min-h-screen place-items-center bg-slate-900 p-8">
      <div className="flex flex-col items-center gap-5 text-center">
        <div className="grid h-14 w-14 place-items-center rounded-xl bg-brand-500 text-2xl font-bold text-white">
          N
        </div>
        <div>
          <h1 className="text-3xl font-bold text-white">NexusAPI Admin</h1>
          <p className="mt-1 text-sm text-slate-400">管理后台 · 仅管理员访问</p>
        </div>
        <Link
          href="/login"
          className="rounded-md bg-brand-500 px-6 py-2.5 text-sm font-medium text-white shadow-lifted hover:bg-brand-400"
        >
          管理员登录 →
        </Link>
      </div>
    </main>
  );
}
