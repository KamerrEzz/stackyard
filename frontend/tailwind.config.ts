import type {Config} from 'tailwindcss'

// v1 ships dark mode only (spec.md §5) — 'class' strategy is unused since
// there is no toggle, but keeping it explicit avoids ever accidentally
// picking up a user's OS light-mode preference via the 'media' strategy.
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
