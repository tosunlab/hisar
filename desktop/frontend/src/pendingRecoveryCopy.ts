const headings: Record<string, string> = {
  signup: "Finish creating your account",
  "replace-secret-key": "Finish replacing your Secret Key",
};
const statuses: Record<string, string | Record<string, string>> = {
  "awaiting-kit": "Save and confirm your new Emergency Kit before Hisar sends this change.",
  prepared: "Your Emergency Kit is confirmed saved. Continue to send this change.",
  "delivery-pending": {
    signup: "Your account is ready. Save and confirm your Emergency Kit to finish.",
    "replace-secret-key": "Your Secret Key was replaced. Save and confirm your new Emergency Kit to finish.",
  },
  "needs-finishing": {
    signup: "Your account was created, but setup did not finish on this device. Continue with the same master password.",
    "replace-secret-key": "Your Secret Key was replaced, but setup did not finish on this device. Continue with the same master password.",
  },
  "manual-unlock": "Sign in with your master password and the Secret Key from your saved Emergency Kit.",
  "outcome-unknown": "Hisar could not confirm whether the server finished this change. Continue to check. Do not start again.",
};
export function recoveryHeading(kind: string): string {
  return headings[kind] ?? "Finish account setup";
}
export function recoveryStatus(kind: string, phase: string): string {
  if (!headings[kind]) return "Finish account setup";
  const status = statuses[phase];
  return typeof status === "string" ? status : status?.[kind] ?? "Finish account setup";
}
export function recoveryHost(origin: string): string {
  try { return new URL(origin).host; } catch { return origin; }
}
