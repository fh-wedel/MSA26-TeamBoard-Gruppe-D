/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Outfit', 'sans-serif'],
        mono: ['"IBM Plex Mono"', 'monospace'],
      },
      colors: {
        bg: {
          0: '#05080F',
          1: '#0A1020',
          2: '#101828',
          3: '#172133',
        },
        border: {
          1: '#1C2D40',
          2: '#253548',
          3: '#2F4561',
        },
        accent: {
          DEFAULT: '#38BDF8',
          dim: '#0EA5E9',
          muted: '#0C4A6E',
        },
        amber: {
          DEFAULT: '#FBBF24',
          dim: '#D97706',
          muted: '#451A03',
        },
        success: '#34D399',
        danger: '#F87171',
        warning: '#FB923C',
        purple: '#A78BFA',
        text: {
          0: '#F0F6FF',
          1: '#8BA4BF',
          2: '#4D6A82',
          3: '#2A3F52',
        },
      },
      animation: {
        'slide-in-right': 'slideInRight 0.2s ease-out',
        'fade-in': 'fadeIn 0.15s ease-out',
        'scale-in': 'scaleIn 0.15s ease-out',
      },
      keyframes: {
        slideInRight: {
          from: { transform: 'translateX(100%)', opacity: '0' },
          to: { transform: 'translateX(0)', opacity: '1' },
        },
        fadeIn: {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        scaleIn: {
          from: { transform: 'scale(0.95)', opacity: '0' },
          to: { transform: 'scale(1)', opacity: '1' },
        },
      },
    },
  },
  plugins: [],
}
