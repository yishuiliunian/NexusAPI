import type { Config } from 'tailwindcss';

// NexusAPI Design Tokens —— 来自 design/tokens.pastel
const tokens = {
  colors: {
    brand: {
      50: '#EFF6FF',
      100: '#DBEAFE',
      400: '#60A5FA',
      500: '#3B82F6',
      600: '#2563EB',
      DEFAULT: '#2563EB',
    },
    slate: {
      50: '#F8FAFC',
      100: '#F1F5F9',
      200: '#E2E8F0',
      300: '#CBD5E1',
      400: '#94A3B8',
      500: '#64748B',
      600: '#475569',
      700: '#334155',
      800: '#1E293B',
      900: '#0F172A',
    },
    success: {
      DEFAULT: '#10B981',
      bg: '#ECFDF5',
    },
    warning: {
      DEFAULT: '#F59E0B',
      bg: '#FFFBEB',
    },
    danger: {
      DEFAULT: '#EF4444',
      bg: '#FEF2F2',
    },
    info: {
      DEFAULT: '#06B6D4',
      bg: '#ECFEFF',
    },
    chart: {
      1: '#2563EB',
      2: '#10B981',
      3: '#F59E0B',
      4: '#8B5CF6',
      5: '#EF4444',
      6: '#06B6D4',
      7: '#EC4899',
      8: '#84CC16',
      9: '#F97316',
      10: '#14B8A6',
    },
  },
  borderRadius: {
    xs: '4px',
    sm: '6px',
    md: '8px',
    lg: '12px',
    xl: '16px',
  },
  boxShadow: {
    subtle: '0 1px 2px 0 rgba(15, 23, 42, 0.08)',
    card: '0 1px 3px 0 rgba(15, 23, 42, 0.08), 0 1px 2px -1px rgba(15, 23, 42, 0.06)',
    elevated: '0 4px 12px -2px rgba(15, 23, 42, 0.1), 0 2px 6px -2px rgba(15, 23, 42, 0.05)',
    lifted: '0 10px 24px -4px rgba(15, 23, 42, 0.15)',
  },
};

const config: Config = {
  content: [
    './app/**/*.{ts,tsx}',
    './src/**/*.{ts,tsx}',
    '../web-shared/src/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: tokens.colors,
      borderRadius: tokens.borderRadius,
      boxShadow: tokens.boxShadow,
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['SF Mono', 'Menlo', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
};

export default config;
