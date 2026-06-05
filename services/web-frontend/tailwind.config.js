/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      colors: {
        surface: '#0F0F23',
        elevated: '#1E1B4B',
        border: '#312E81',
      },
    },
  },
  plugins: [],
}
