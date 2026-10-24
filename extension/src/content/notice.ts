import { noticeMilliseconds } from "../shared/messages";

const hostTagName = "hisar-notice";

const styles = `
:host { all: initial !important; position: fixed !important; inset: 16px 16px auto auto !important; z-index: 2147483647 !important; pointer-events: none !important; }
.notice {
  box-sizing: border-box; max-width: 320px; padding: 10px 14px; border: 1px solid #30343a; border-radius: 10px;
  background: #111214; color: #eceef1; font: 13px/1.4 system-ui, sans-serif; box-shadow: 0 12px 32px rgb(0 0 0 / 45%);
}
`;

// showNotice tells the user, in the page, why a fill from the keyboard shortcut or the right-click menu did nothing. It
// shows only Hisar's own sentence, never anything from a vault item, and goes away by itself.
export function showNotice(document: Document, text: string): void {
  document.querySelector(hostTagName)?.remove();
  const host = document.createElement(hostTagName);
  const shadowRoot = host.attachShadow({ mode: "closed" });
  const style = document.createElement("style");
  style.textContent = styles;
  const notice = document.createElement("div");
  notice.className = "notice";
  notice.setAttribute("role", "status");
  notice.textContent = `Hisar: ${text}`;
  shadowRoot.append(style, notice);
  const isPopoverSupported = typeof host.showPopover === "function";
  if (isPopoverSupported) host.setAttribute("popover", "manual");
  document.documentElement.append(host);
  if (isPopoverSupported) host.showPopover();
  setTimeout(() => host.remove(), noticeMilliseconds);
}
