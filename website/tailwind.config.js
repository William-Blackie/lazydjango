/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx,js,jsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#0f172a",
        mist: "#e2e8f0",
        mint: "#7dd3fc",
        leaf: "#86efac",
        amber: "#fcd34d",
        ember: "#fb7185",
      },
      boxShadow: {
        glass: "0 10px 30px rgba(2, 6, 23, 0.35)",
      },
      borderRadius: {
        xl2: "1rem",
      },
      fontFamily: {
        sans: ["Space Grotesk", "system-ui", "sans-serif"],
        mono: ["IBM Plex Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
    },
  },
  plugins: [],
}
