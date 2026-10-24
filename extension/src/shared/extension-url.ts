// isExtensionPageUrl reports whether url is the page at pathname of an extension in this browser: chrome-extension: in
// Chrome, moz-extension: in Firefox, read from extensionBaseUrl (chrome.runtime.getURL("")). Only the scheme is compared,
// because Chrome serves the inline menu and the passkey prompt from a dynamic ID (use_dynamic_url) that differs from the
// extension's own; callers that need the sender to be this extension check its ID as well.
export function isExtensionPageUrl(url: string | undefined, extensionBaseUrl: string, pathname?: string): boolean {
  try {
    const parsed = new URL(url ?? "");
    if (parsed.protocol !== new URL(extensionBaseUrl).protocol) return false;
    return pathname === undefined || parsed.pathname === pathname;
  } catch {
    return false;
  }
}
