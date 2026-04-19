'use client';

// 封装 Recharts 通用图表组件。
// Dashboard / Admin Overview 复用。

import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

export const CHART_COLORS = [
  '#2563eb', // blue-600
  '#16a34a', // green-600
  '#ea580c', // orange-600
  '#9333ea', // purple-600
  '#dc2626', // red-600
  '#0891b2', // cyan-600
  '#ca8a04', // yellow-600
  '#4f46e5', // indigo-600
  '#be185d', // pink-600
  '#0f766e', // teal-700
];

// ---------- LegacyStatCard（保留旧调用；新代码用 ui/card 的 StatCard） ----------

export interface LegacyStatCardProps {
  label: string;
  value: string;
  hint?: string;
  accent?: 'blue' | 'green' | 'orange' | 'red' | 'purple';
}

const accentMap = {
  blue: 'border-l-blue-500',
  green: 'border-l-green-500',
  orange: 'border-l-orange-500',
  red: 'border-l-red-500',
  purple: 'border-l-purple-500',
};

/** @deprecated 使用 ui/card 的 StatCard */
export function LegacyStatCard({ label, value, hint, accent = 'blue' }: LegacyStatCardProps) {
  return (
    <div className={`rounded border border-l-4 bg-white p-4 shadow-sm ${accentMap[accent]}`}>
      <p className="text-xs font-medium text-gray-500">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-gray-900">{value}</p>
      {hint && <p className="mt-1 text-xs text-gray-400">{hint}</p>}
    </div>
  );
}

// ---------- 折线/面积图：按天趋势 ----------

export interface TrendPoint {
  date: string;
  [key: string]: number | string;
}

export interface TrendChartProps {
  data: TrendPoint[];
  series: { key: string; label: string; color?: string }[];
  height?: number;
  type?: 'line' | 'area';
  /** Y 轴数值格式化。例如 (n) => (n / 1_000_000).toFixed(2) 把 micro 转元。 */
  yFormat?: (v: number) => string;
}

// Recharts 3.x Tooltip formatter 类型严格；统一收敛到 unknown → string 适配。
function wrapFmt(fn?: (v: number) => string) {
  if (!fn) return undefined;
  return (v: unknown) => (typeof v === 'number' ? fn(v) : String(v ?? ''));
}

export function TrendChart({ data, series, height = 240, type = 'area', yFormat }: TrendChartProps) {
  const Component = type === 'area' ? AreaChart : LineChart;
  return (
    <ResponsiveContainer width="100%" height={height}>
      <Component data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
        <XAxis dataKey="date" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} tickFormatter={yFormat} />
        <Tooltip
          contentStyle={{ fontSize: 12 }}
          formatter={wrapFmt(yFormat) as never}
        />
        <Legend wrapperStyle={{ fontSize: 12 }} />
        {series.map((s, i) => {
          const color = s.color ?? CHART_COLORS[i % CHART_COLORS.length];
          if (type === 'area') {
            return (
              <Area
                key={s.key}
                type="monotone"
                dataKey={s.key}
                name={s.label}
                stroke={color}
                fill={color}
                fillOpacity={0.2}
                strokeWidth={2}
              />
            );
          }
          return (
            <Line
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.label}
              stroke={color}
              strokeWidth={2}
              dot={{ r: 3 }}
            />
          );
        })}
      </Component>
    </ResponsiveContainer>
  );
}

// ---------- 柱状图：按类别 ----------

export interface BarPoint {
  name: string;
  value: number;
}

export interface BarChartProps {
  data: BarPoint[];
  color?: string;
  height?: number;
  yFormat?: (v: number) => string;
}

export function SimpleBarChart({ data, color = CHART_COLORS[0], height = 240, yFormat }: BarChartProps) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={{ top: 10, right: 10, left: 0, bottom: 40 }}>
        <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} angle={-30} textAnchor="end" interval={0} />
        <YAxis tick={{ fontSize: 11 }} tickFormatter={yFormat} />
        <Tooltip
          contentStyle={{ fontSize: 12 }}
          formatter={wrapFmt(yFormat) as never}
        />
        <Bar dataKey="value" fill={color} radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  );
}

// ---------- 环形图：比例 ----------

export interface PiePoint {
  name: string;
  value: number;
}

export interface PieChartProps {
  data: PiePoint[];
  height?: number;
  valueFormat?: (v: number) => string;
}

export function SimplePieChart({ data, height = 240, valueFormat }: PieChartProps) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <PieChart>
        <Tooltip
          contentStyle={{ fontSize: 12 }}
          formatter={wrapFmt(valueFormat) as never}
        />
        <Legend wrapperStyle={{ fontSize: 11 }} />
        <Pie data={data} dataKey="value" nameKey="name" innerRadius={50} outerRadius={80} paddingAngle={2}>
          {data.map((_, i) => (
            <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
          ))}
        </Pie>
      </PieChart>
    </ResponsiveContainer>
  );
}