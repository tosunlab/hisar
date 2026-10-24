// PageSettings are this browser's settings for what Hisar does in web pages.
export interface PageSettings {
  isInlineMenuEnabled: boolean;
  isInlineMenuOpenedOnFocus: boolean;
  // isFillDiagnosticsEnabled writes what Hisar found in a page, and what a fill did, to the page's console.
  isFillDiagnosticsEnabled: boolean;
}

export const defaultPageSettings: PageSettings = {
  isInlineMenuEnabled: true,
  isInlineMenuOpenedOnFocus: true,
  isFillDiagnosticsEnabled: false,
};

export const allPageSettingKey = Object.keys(defaultPageSettings) as (keyof PageSettings)[];

export interface SettingsArea {
  get(allKey: string[]): Promise<Record<string, unknown>>;
  set(allEntry: Record<string, unknown>): Promise<void>;
}

export function parsePageSettings(stored: Record<string, unknown>): PageSettings {
  const settings = { ...defaultPageSettings };
  for (const key of allPageSettingKey) {
    if (typeof stored[key] === "boolean") settings[key] = stored[key];
  }
  return settings;
}

export async function readPageSettings(area: SettingsArea): Promise<PageSettings> {
  return parsePageSettings(await area.get(allPageSettingKey));
}

// appFeaturesKey holds the features the paired app announced in its last hello, so a page knows whether to offer what
// only a newer app fills, such as addresses, before it asks.
export const appFeaturesKey = "appFeatures";

export async function readAppFeatures(area: SettingsArea): Promise<string[]> {
  const stored = (await area.get([appFeaturesKey]))[appFeaturesKey];
  return Array.isArray(stored) ? stored.filter((feature): feature is string => typeof feature === "string") : [];
}

export async function writePageSetting(area: SettingsArea, key: keyof PageSettings, value: boolean): Promise<void> {
  await area.set({ [key]: value });
}
