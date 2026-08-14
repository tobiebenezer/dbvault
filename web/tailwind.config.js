/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./src/**/*.{js,jsx,html}",
    "./index.html"
  ],
  theme: {
    extend: {
      colors: {
        canvas: '#f8fafc',
        panel: '#ffffff',
        subtle: '#f8fafc',
        inset: '#f1f5f9',
        ink: {
          DEFAULT: '#0f172a',
          soft: '#334155',
          muted: '#64748b',
          light: '#94a3b8'
        },
        line: {
          DEFAULT: '#e2e8f0',
          strong: '#cbd5e1'
        },
        brand: {
          DEFAULT: '#0f766e',
          dark: '#115e59',
          soft: '#f0fdfa'
        },
        darkbtn: {
          DEFAULT: '#0f172a',
          hover: '#1e293b'
        },
        status: {
          success: '#16a34a',
          warning: '#d97706',
          danger: '#dc2626'
        }
      },
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      }
    }
  },
  plugins: []
};
