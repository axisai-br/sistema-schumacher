import { heroui } from "@heroui/react";

/** @type {import('tailwindcss').Config} */
export default {
    content: [
        "./index.html",
        "./src/**/*.{js,ts,jsx,tsx}",
        "./node_modules/@heroui/theme/dist/**/*.{js,ts,jsx,tsx}",
    ],
    theme: {
        extend: {
            colors: {
                // Schumacher Tur official palette
                brand: {
                    gunship: '#414951',
                    ember: '#DF674A',
                    'blue-grey': '#A0A9AF',
                    wasp: '#E8B100',
                    inca: '#A08B63',
                    lynx: '#F0F5F7',
                },
                // Light mode palette
                gold: {
                    50: '#FDF8E7',
                    100: '#F9EEC5',
                    200: '#F3DC8C',
                    300: '#E8C84D',
                    400: '#D4AF37', // Principal
                    500: '#C5A028',
                    600: '#B8941F',
                    700: '#9A7A1A',
                    800: '#7D6315',
                    900: '#5F4B10',
                },
                light: {
                    50: '#FFFFFF',
                    100: '#FAFAFA',
                    200: '#F5F5F5',
                    300: '#EBEBEB',
                    400: '#E0E0E0',
                },
                dark: {
                    900: '#0A0A0A',
                    800: '#1A1A1A',
                    700: '#2A2A2A',
                    600: '#3A3A3A',
                    500: '#4A4A4A',
                },
            },
            fontFamily: {
                heading: ['Franie', 'Outfit', 'Inter', 'sans-serif'],
                body: ['Franie', 'Outfit', 'Inter', 'sans-serif'],
            },
            backgroundImage: {
                'gradient-gold': 'linear-gradient(135deg, #D4AF37 0%, #F3DC8C 50%, #D4AF37 100%)',
                'gradient-gold-subtle': 'linear-gradient(135deg, rgba(212,175,55,0.1) 0%, rgba(243,220,140,0.05) 100%)',
            },
            boxShadow: {
                'gold': '0 4px 20px rgba(212, 175, 55, 0.25)',
                'gold-lg': '0 8px 40px rgba(212, 175, 55, 0.35)',
                'soft': '0 4px 20px rgba(0, 0, 0, 0.08)',
                'soft-lg': '0 8px 40px rgba(0, 0, 0, 0.12)',
            },
        },
    },
    darkMode: "class",
    plugins: [heroui({
        themes: {
            light: {
                colors: {
                    primary: {
                        DEFAULT: "#DF674A",
                        foreground: "#000000",
                    },
                    focus: "#DF674A",
                },
            },
        },
    })],
}
