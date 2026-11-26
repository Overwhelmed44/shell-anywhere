import { createContext } from "react";
import type { Theme } from "./types";

export type AppSettings = {
  theme: Theme;
  connectionTimeoutSeconds: number;
  terminalFontSize: number;
  terminalScrollback: number;
  terminalCursorBlink: boolean;
};

export const defaultSettings: AppSettings = {
  theme: "dark",
  connectionTimeoutSeconds: 12,
  terminalFontSize: 13,
  terminalScrollback: 5000,
  terminalCursorBlink: true,
};

export const SettingsContext = createContext(defaultSettings);
