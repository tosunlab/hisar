export interface InlineMenuSettings {
  isInlineMenuEnabled: boolean;
  isInlineMenuOpenedOnFocus: boolean;
}

export const defaultInlineMenuSettings: InlineMenuSettings = {
  isInlineMenuEnabled: true,
  isInlineMenuOpenedOnFocus: true,
};

export const allInlineMenuSettingKey = Object.keys(defaultInlineMenuSettings) as (keyof InlineMenuSettings)[];

export interface SettingsArea {
  get(allKey: string[]): Promise<Record<string, unknown>>;
  set(allEntry: Record<string, unknown>): Promise<void>;
}

export function parseInlineMenuSettings(stored: Record<string, unknown>): InlineMenuSettings {
  const settings = { ...defaultInlineMenuSettings };
  for (const key of allInlineMenuSettingKey) {
    if (typeof stored[key] === "boolean") settings[key] = stored[key];
  }
  return settings;
}

export async function readInlineMenuSettings(area: SettingsArea): Promise<InlineMenuSettings> {
  return parseInlineMenuSettings(await area.get(allInlineMenuSettingKey));
}

// appFeaturesKey holds the features the paired app announced in its last hello, so a page knows whether to offer what
// only a newer app fills, such as addresses, before it asks.
export const appFeaturesKey = "appFeatures";

export async function readAppFeatures(area: SettingsArea): Promise<string[]> {
  const stored = (await area.get([appFeaturesKey]))[appFeaturesKey];
  return Array.isArray(stored) ? stored.filter((feature): feature is string => typeof feature === "string") : [];
}

export async function writeInlineMenuSetting(area: SettingsArea, key: keyof InlineMenuSettings, value: boolean): Promise<void> {
  await area.set({ [key]: value });
}
