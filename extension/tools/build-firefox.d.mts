export const firefoxExtensionID: string;
export const firefoxMinimumVersion: string;
export const firefoxUpdateURL: string;
export function toFirefoxManifest<Manifest extends Record<string, unknown>>(chromeManifest: Manifest): Record<string, unknown>;
export function buildFirefox(extensionDirectory: string): string;
