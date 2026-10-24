export const extensionID: string;
export const firefoxExtensionID: string;
export const hostName: string;
export function getDefaultUserDataDirectory(browser?: string): string;
export function registerFakeHost(options: {
  userDataDirectory: string;
  stateFile: string;
  hostDirectory: string;
  shouldResetState?: boolean;
}): { manifestPath: string; launcherPath: string; stateFile: string };
export function getFirefoxManifestsDirectory(): string;
export function registerFakeFirefoxHost(options: {
  manifestsDirectory: string;
  stateFile: string;
  hostDirectory: string;
  shouldResetState?: boolean;
}): { manifestPath: string; launcherPath: string; stateFile: string };
