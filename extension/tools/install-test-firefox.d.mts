export const testFirefoxBuildID: string;
export const testGeckodriverVersion: string;
export function installTestFirefox(buildId?: string): Promise<{ firefoxPath: string; geckodriverPath: string }>;
