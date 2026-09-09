/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#eefaf6',
          100: '#d9f3ea',
          200: '#b5e7d7',
          300: '#83d4bd',
          400: '#46b99a',
          500: '#239b7e',
          600: '#087f68',
          700: '#086654',
          800: '#095144',
          900: '#073f36',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
    },
  },
  plugins: [],
}
