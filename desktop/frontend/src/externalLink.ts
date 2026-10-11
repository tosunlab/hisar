import { api } from "./api";

// openExternalLink sends clicks on http(s) links with target="_blank" to the default browser instead of the app's web view.
export function openExternalLink(event: MouseEvent): void {
  const target = event.target;
  const anchor = target instanceof Element ? target.closest("a[target='_blank']") : null;
  if (!(anchor instanceof HTMLAnchorElement) || !/^https?:$/.test(anchor.protocol)) {
    return;
  }
  event.preventDefault();
  api.openURL(anchor.href).catch(() => undefined);
}
