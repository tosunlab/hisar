// Fill diagnostics, turned on in the popup's settings, write what Hisar found in the page and what a fill did to the
// page's console, to explain a fill that did nothing. They name fields, never their values.
let isEnabled = false;

export function setFillDiagnosticsEnabled(isOn: boolean): void {
  isEnabled = isOn;
}

export function describeFieldForDiagnostics(field: HTMLInputElement | undefined): string {
  if (!field) return "none";
  const name = field.id ? `#${field.id}` : field.name ? `[name=${field.name}]` : "(unnamed)";
  return `input${name} type=${field.type}${field.autocomplete ? ` autocomplete=${field.autocomplete}` : ""}`;
}

export function logFillDiagnostics(event: string, details: Record<string, unknown>): void {
  if (isEnabled) console.debug(`[Hisar] ${event}`, details);
}
