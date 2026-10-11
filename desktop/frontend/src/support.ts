import type { Device, UpdateStatus } from "./api";

export const supportEmailAddress = "support@tryhisar.com";

export interface DiagnosticsInput {
  updateStatus: UpdateStatus | null;
  server: string;
  allDevice: Device[];
}

export function formatAppVersion(updateStatus: UpdateStatus | null): string {
  if (!updateStatus?.version) {
    return "unknown";
  }
  return updateStatus.buildNumber ? `${updateStatus.version} (${updateStatus.buildNumber})` : updateStatus.version;
}

// buildDiagnostics lists only versions, the server and this device's ID: never secrets, names or other people's emails.
export function buildDiagnostics({ updateStatus, server, allDevice }: DiagnosticsInput): string {
  const currentDevice = allDevice.find((device) => device.isCurrent);
  return [
    `Hisar for Mac ${formatAppVersion(updateStatus)}`,
    `System: ${updateStatus?.operatingSystem || "unknown"}`,
    `Server: ${server || "not signed in"}`,
    `Device ID: ${currentDevice?.id ?? "unknown"}`,
  ].join("\n");
}
