const defaultTheme = require('tailwindcss/defaultTheme')

module.exports = {
  purge: [
    './src/**/*.html',
    './src/**/*.tsx',
  ],
  darkMode: false, // or 'media' or 'class'
  theme: {
    extend: {
      fontFamily: {
        heading: ['Poppins', ...defaultTheme.fontFamily.sans],
        body: ['Noto Sans JP', 'Noto Sans', ...defaultTheme.fontFamily.serif],
        mono: [...defaultTheme.fontFamily.mono],
      }
    },
  },
  variants: {
    extend: {},
  },
  plugins: [],
}
