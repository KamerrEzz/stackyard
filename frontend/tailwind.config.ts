import type {Config} from 'tailwindcss'

export default {
    darkMode: 'class',
    content: ['./index.html', './src/**/*.{ts,tsx}'],
    theme: {
        extend: {
            colors: {
                ink: {
                    950: '#0a0e17',
                    900: '#10151f',
                    850: '#141a26',
                    800: '#1b2330',
                    700: '#2a3444',
                    600: '#3d4a5e',
                    400: '#7c8aa0',
                    200: '#c4cddb',
                    100: '#e4e8ef',
                },
                brass: {
                    600: '#a8722c',
                    500: '#c9903f',
                    400: '#e0ab5c',
                },
            },
            fontFamily: {
                sans: ['Nunito', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'Roboto', 'sans-serif'],
                mono: ['ui-monospace', '"Cascadia Code"', '"Segoe UI Mono"', 'Consolas', 'monospace'],
            },
        },
    },
    plugins: [],
} satisfies Config
