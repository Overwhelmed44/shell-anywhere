import type { CSSProperties } from "react";
import type { ITheme } from "@xterm/xterm";

export const themeIds = ["dark", "midnight", "forest", "rose", "arctic", "light", "sand", "paper"] as const;
export type Theme = typeof themeIds[number];

type Palette = {
  bg: string; sidebar: string; header: string; card: string; surface: string;
  raised: string; hover: string; text: string; secondary: string; muted: string;
  faint: string; border: string; borderStrong: string; accent: string;
  accentHover: string; accentText: string; accentSoft: string; accentBorder: string;
  onAccent: string; success: string; danger: string; warning: string;
  terminalBackground?: string; terminalForeground?: string;
};

type ThemeDefinition = {
  id: Theme;
  label: string;
  description: string;
  mode: "dark" | "light";
  variables: CSSProperties & Record<`--color-${string}`, string>;
  terminal: ITheme;
};

function createTheme(id: Theme, label: string, description: string, mode: "dark" | "light", p: Palette): ThemeDefinition {
  const dark = mode === "dark";
  const terminal: ITheme = {
    background: p.terminalBackground ?? p.bg,
    foreground: p.terminalForeground ?? p.text,
    cursor: p.accentText,
    cursorAccent: p.bg,
    selectionBackground: `${p.accentText}44`,
    selectionForeground: p.text,
    black: dark ? p.raised : p.text,
    red: p.danger,
    green: p.success,
    yellow: p.warning,
    blue: dark ? "#8fb8f8" : "#3559b6",
    magenta: dark ? "#c7a8ee" : "#6e50a8",
    cyan: dark ? "#8edbd5" : "#126d82",
    white: dark ? p.text : p.secondary,
    brightBlack: p.muted,
    brightRed: p.danger,
    brightGreen: p.success,
    brightYellow: p.warning,
    brightBlue: dark ? "#adcfff" : "#244a9d",
    brightMagenta: dark ? "#e0c3ff" : "#58398c",
    brightCyan: dark ? "#b0f1ea" : "#07586d",
    brightWhite: dark ? "#ffffff" : p.text,
  };
  const variables: ThemeDefinition["variables"] = {
    "--color-bg": p.bg,
    "--color-sidebar": p.sidebar,
    "--color-header": p.header,
    "--color-card": p.card,
    "--color-surface": p.surface,
    "--color-surface-raised": p.raised,
    "--color-input": p.bg,
    "--color-modal": p.card,
    "--color-hover": p.hover,
    "--color-active": p.surface,
    "--color-text": p.text,
    "--color-text-strong": p.text,
    "--color-text-secondary": p.secondary,
    "--color-text-muted": p.muted,
    "--color-text-faint": p.faint,
    "--color-border": p.border,
    "--color-border-strong": p.borderStrong,
    "--color-accent": p.accent,
    "--color-accent-hover": p.accentHover,
    "--color-accent-text": p.accentText,
    "--color-accent-soft": p.accentSoft,
    "--color-accent-border": p.accentBorder,
    "--color-on-accent": p.onAccent,
    "--color-success": p.success,
    "--color-danger": p.danger,
    "--color-warning": p.warning,
    "--color-error-bg": dark ? "#482e34" : "#fff0f2",
    "--color-error-border": dark ? "#87535b" : "#efb7c0",
    "--color-error-text": dark ? "#ffc2c5" : "#9f3443",
    "--color-overlay": dark ? "#05080bc9" : "#1d214966",
    "--color-shadow": dark ? "#00000099" : "#2024432b",
    "--color-shadow-soft": dark ? "#00000066" : "#20244322",
    "--color-focus-ring": `${p.accentText}33`,
    "--color-accent-glow": `${p.accent}44`,
    "--color-accent-glow-soft": `${p.accent}1c`,
    "--color-security-bg": `linear-gradient(145deg, ${p.accentSoft}, ${p.card})`,
    "--color-fingerprint-bg": p.bg,
    "--color-toast-bg": p.accentSoft,
    "--color-toast-text": p.text,
    "--color-toast-border": p.accentBorder,
    "--color-illustration-bg": p.bg,
    "--color-illustration-text": p.text,
    "--color-illustration-border": p.accentBorder,
    "--color-illustration-muted": p.muted,
    "--color-illustration-accent": p.accent,
    "--color-terminal-bg": terminal.background!,
    "--color-terminal-toolbar": p.surface,
    "--color-terminal-border": p.borderStrong,
  };
  return { id, label, description, mode, variables, terminal };
}

export const themes: Record<Theme, ThemeDefinition> = {
  dark: createTheme("dark", "Dark", "Classic charcoal and lavender", "dark", {
    bg: "#10141b", sidebar: "#141a22", header: "#121820", card: "#171e27", surface: "#19212a",
    raised: "#212b35", hover: "#28343e", text: "#e8eef5", secondary: "#a7b7c4", muted: "#8294a2",
    faint: "#647684", border: "#26303b", borderStrong: "#3a4853", accent: "#c1c1ff",
    accentHover: "#d8d8ff", accentText: "#c1c1ff", accentSoft: "#30304b", accentBorder: "#6565a3",
    onAccent: "#22203d", success: "#71dbae", danger: "#ef999b", warning: "#ecc777", terminalForeground: "#dbe5ef",
  }),
  midnight: createTheme("midnight", "Midnight", "Deep navy with electric blue", "dark", {
    bg: "#0b1020", sidebar: "#0e1529", header: "#0c1325", card: "#111b31", surface: "#14213b",
    raised: "#1b2b48", hover: "#223555", text: "#e3edff", secondary: "#b0c2de", muted: "#8a9fbd",
    faint: "#667e9f", border: "#23334e", borderStrong: "#3a5275", accent: "#8db9ff",
    accentHover: "#b1d0ff", accentText: "#9bc2ff", accentSoft: "#1d3156", accentBorder: "#5079b2",
    onAccent: "#0d2343", success: "#83d8ba", danger: "#f497a4", warning: "#f2ce86",
  }),
  forest: createTheme("forest", "Forest", "Evergreen with soft mint", "dark", {
    bg: "#101a16", sidebar: "#14211b", header: "#122019", card: "#18271f", surface: "#1b2e25",
    raised: "#243a2f", hover: "#2b4437", text: "#e2eee6", secondary: "#b1c7b9", muted: "#8ba997",
    faint: "#6b8978", border: "#2a4033", borderStrong: "#425e4c", accent: "#a2d9b5",
    accentHover: "#c1e8cd", accentText: "#a2d9b5", accentSoft: "#284634", accentBorder: "#558365",
    onAccent: "#193423", success: "#93d8ab", danger: "#f0a5a0", warning: "#e6ce8b",
  }),
  rose: createTheme("rose", "Rose", "Plum shadows and dusty pink", "dark", {
    bg: "#1b141d", sidebar: "#221925", header: "#201722", card: "#2a1f2d", surface: "#302335",
    raised: "#3b2b41", hover: "#45314a", text: "#f3e7f0", secondary: "#cfb7cc", muted: "#b296ae",
    faint: "#8d7189", border: "#402c43", borderStrong: "#604563", accent: "#edb4ce",
    accentHover: "#f7cede", accentText: "#edb4ce", accentSoft: "#4a2b40", accentBorder: "#98617f",
    onAccent: "#3b2033", success: "#a1d7b7", danger: "#f5a3ad", warning: "#eacb94",
  }),
  arctic: createTheme("arctic", "Arctic", "Cool slate with icy cyan", "dark", {
    bg: "#242c38", sidebar: "#293341", header: "#27303d", card: "#2e3948", surface: "#334151",
    raised: "#3d4c5f", hover: "#47586c", text: "#edf3fa", secondary: "#c3cfde", muted: "#a4b4c7",
    faint: "#8496ad", border: "#425167", borderStrong: "#5b6e87", accent: "#98d3df",
    accentHover: "#b6e4ed", accentText: "#a5e0eb", accentSoft: "#354f61", accentBorder: "#6c9dae",
    onAccent: "#203c49", success: "#b2d5a4", danger: "#eba8ad", warning: "#ebcf9b",
  }),
  light: createTheme("light", "Light", "Bright surfaces and lavender", "light", {
    bg: "#f7f8fd", sidebar: "#eff1fa", header: "#ffffff", card: "#ffffff", surface: "#f7f8fd",
    raised: "#eceef8", hover: "#e5e8f3", text: "#20243a", secondary: "#59627b", muted: "#667088",
    faint: "#8990a5", border: "#dce0ed", borderStrong: "#bfc6d8", accent: "#c1c1ff",
    accentHover: "#adabfb", accentText: "#514a98", accentSoft: "#ecebff", accentBorder: "#aaa9ed",
    onAccent: "#22203d", success: "#18805b", danger: "#b54251", warning: "#a2701d", terminalBackground: "#ffffff",
  }),
  sand: createTheme("sand", "Sand", "Warm cream with golden accents", "light", {
    bg: "#faf6ef", sidebar: "#f2ebdf", header: "#fffdf8", card: "#fffdf8", surface: "#f7f0e5",
    raised: "#eee4d5", hover: "#e8dcc9", text: "#3b3024", secondary: "#6d5b46", muted: "#7c6b56",
    faint: "#9a8770", border: "#e5d8c5", borderStrong: "#c9b79d", accent: "#e8c894",
    accentHover: "#dbb87f", accentText: "#805715", accentSoft: "#f4e5ca", accentBorder: "#c8a367",
    onAccent: "#463217", success: "#37734d", danger: "#b34745", warning: "#8a611c", terminalBackground: "#fffaf2",
  }),
  paper: createTheme("paper", "Paper", "Quiet neutrals with graphite", "light", {
    bg: "#f4f4f2", sidebar: "#eaeae7", header: "#fdfdfb", card: "#fdfdfb", surface: "#eeeeeb",
    raised: "#e5e5e1", hover: "#dcdcd7", text: "#282d2a", secondary: "#555e58", muted: "#626c65",
    faint: "#888f88", border: "#dadbd6", borderStrong: "#b7bcb5", accent: "#c5d3c8",
    accentHover: "#b3c3b7", accentText: "#405d49", accentSoft: "#e0e8e0", accentBorder: "#9aaf9e",
    onAccent: "#283e2f", success: "#356d46", danger: "#ae4749", warning: "#886825", terminalBackground: "#fdfdfb",
  }),
};

// Preserve the established ANSI colors of the original themes.
Object.assign(themes.dark.terminal, { selectionBackground: "#7772df88", black: "#202936", red: "#f58d91", green: "#90d8a5", yellow: "#f2ce86", white: "#dbe5ef", brightBlack: "#778390" });
Object.assign(themes.light.terminal, { selectionBackground: "#c1c1ff99", black: "#20243a", yellow: "#9b711d", brightBlack: "#737a8c" });

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && Object.hasOwn(themes, value);
}

export function applyTheme(id: Theme) {
  const theme = themes[id];
  const root = document.documentElement;
  root.dataset.theme = id;
  root.dataset.colorScheme = theme.mode;
  root.style.colorScheme = theme.mode;
  // Original themes use their existing CSS palettes. Clear every override on
  // each change so switching between dark and light variants never leaks colors.
  for (const [property, value] of Object.entries(theme.variables)) {
    if (id === "dark" || id === "light") root.style.removeProperty(property);
    else root.style.setProperty(property, value);
  }
}
