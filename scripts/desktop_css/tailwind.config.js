/** Tailwind v3 config for the embedded APW Desktop UI. Built by build.py. */
module.exports = {
  darkMode: 'class',
  content: {
    relative: true,
    files: ['../../internal/desktop/ui.go'],
  },
  theme: {
    extend: {
      colors: {
        obsidian: '#0B0D11',
        slatepanel: '#11141B',
        cardbg: '#161A23',
        cardhover: '#1C212D',
        borderdim: 'rgba(255, 255, 255, 0.08)',
        accent: '#E07A5F',
        accenthover: '#EB8C73',
      },
      fontFamily: {
        sans: ['Inter', 'Noto Sans Thai', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'Cascadia Code', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
};
