import { reactive } from "vue";
import { Events } from "@wailsio/runtime";
import { api, getErrorMessage, type Theme } from "./api";

export type Appearance = "system" | "light" | "dark";
export type ThemeColor = "neutral" | "blue" | "purple" | "green";

export const allAppearanceOption: { id: Appearance; label: string }[] = [
  { id: "system", label: "System" },
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
];

export const allThemeColorOption: { id: ThemeColor; label: string }[] = [
  { id: "neutral", label: "Neutral" },
  { id: "blue", label: "Blue" },
  { id: "purple", label: "Purple" },
  { id: "green", label: "Green" },
];

const defaultTheme: Theme = { appearance: "system", color: "neutral" };
// Only a first-paint cache for new windows; desktop.json holds the setting.
const themeStorageKey = "hisar.theme";

export const themeStore = reactive({
  appearance: defaultTheme.appearance as Appearance,
  color: defaultTheme.color as ThemeColor,
  errorMessage: "",
});

function isAppearance(value: unknown): value is Appearance {
  return allAppearanceOption.some((option) => option.id === value);
}

function isThemeColor(value: unknown): value is ThemeColor {
  return allThemeColorOption.some((option) => option.id === value);
}

export function applyTheme(theme: Theme): void {
  themeStore.appearance = isAppearance(theme.appearance) ? theme.appearance : "system";
  themeStore.color = isThemeColor(theme.color) ? theme.color : "neutral";
  document.documentElement.dataset.appearance = themeStore.appearance;
  document.documentElement.dataset.color = themeStore.color;
  try {
    window.localStorage.setItem(themeStorageKey, JSON.stringify({ appearance: themeStore.appearance, color: themeStore.color }));
  } catch {
    return;
  }
}

export function applyCachedTheme(): void {
  try {
    applyTheme(JSON.parse(window.localStorage.getItem(themeStorageKey) ?? "null") ?? defaultTheme);
  } catch {
    applyTheme(defaultTheme);
  }
}

// startThemeSync loads the saved theme and follows changes made in any window; it returns the stop function.
export function startThemeSync(): () => void {
  const removeListener = Events.On("theme-changed", (event) => applyTheme(event.data));
  api
    .getTheme()
    .then(applyTheme)
    .catch(() => undefined);
  return removeListener;
}

// chooseTheme shows the theme at once and saves it; a failed save goes back to the theme in use.
export async function chooseTheme(appearance: Appearance, color: ThemeColor): Promise<void> {
  const previousTheme: Theme = { appearance: themeStore.appearance, color: themeStore.color };
  themeStore.errorMessage = "";
  applyTheme({ appearance, color });
  try {
    applyTheme(await api.setTheme(appearance, color));
  } catch (error) {
    applyTheme(previousTheme);
    themeStore.errorMessage = getErrorMessage(error);
  }
}
