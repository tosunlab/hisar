import type { SaveOfferAction, SaveOfferView } from "../shared/messages";

export interface SaveBar {
  host: HTMLElement;
  shadowRoot: ShadowRoot;
  remove: () => void;
}

export type SaveBarActionHandler = (action: SaveOfferAction) => Promise<{ ok: boolean; message?: string }>;

const hostTagName = "hisar-save-bar";

const styles = `
:host { all: initial; }
.bar {
  position: fixed; top: 12px; right: 12px; z-index: 2147483647;
  width: min(380px, calc(100vw - 24px)); box-sizing: border-box;
  padding: 14px; border: 1px solid #30343a; border-radius: 12px;
  background: #111214; color: #e8eaed; box-shadow: 0 12px 32px rgb(0 0 0 / 50%);
  font: 13px/1.45 -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased; color-scheme: dark;
}
.header { display: flex; align-items: center; gap: 10px; }
.title { flex: 1; margin: 0; font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
.detail { margin: 6px 0 0 34px; color: #b4b9c0; font-size: 12px; overflow-wrap: anywhere; }
.detail strong { color: #e8eaed; font-weight: 500; }
.actions { display: flex; gap: 8px; margin-top: 12px; justify-content: flex-end; flex-wrap: wrap; }
button {
  display: inline-flex; align-items: center; justify-content: center;
  padding: 6px 11px; border: 1px solid #30343a; border-radius: 8px;
  background: #1c1f23; color: #e8eaed; font: inherit; font-weight: 500; cursor: pointer;
}
button:hover:not(:disabled) { background: #1f2226; border-color: #3b4047; }
button:focus-visible { outline: 2px solid #8a95a5; outline-offset: 1px; }
button:disabled { opacity: 0.45; cursor: default; }
button.primary { border-color: #eceef1; background: #eceef1; color: #0b0c0e; font-weight: 600; }
button.primary:hover:not(:disabled) { background: #ffffff; border-color: #ffffff; }
button.ghost { border-color: transparent; background: transparent; color: #b4b9c0; }
button.ghost:hover:not(:disabled) { background: #1f2226; color: #e8eaed; }
button.close { padding: 4px; }
.message { margin: 10px 0 0 34px; font-size: 12px; color: #b4b9c0; }
.message.error { color: #f2a7a9; }
`;

const logoSvg = `<svg width="24" height="24" viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="8" fill="#f2f3f5"/><path d="M8 24.5V10.5h3V7.5h2.5v3h5V7.5H21v3h3v14h-6v-5a2 2 0 0 0-4 0v5z" fill="#0b0c0e"/></svg>`;
const closeSvg = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>`;

function createElement<Tag extends keyof HTMLElementTagNameMap>(
  document: Document,
  tagName: Tag,
  className?: string,
  text?: string,
): HTMLElementTagNameMap[Tag] {
  const element = document.createElement(tagName);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

export function describeSaveOffer(offer: SaveOfferView): { title: string; primaryLabel: string } {
  if (offer.kind === "update") {
    return { title: `Update the password for ${offer.title || offer.host} in Hisar?`, primaryLabel: "Update" };
  }
  return { title: `Save this login for ${offer.host} in Hisar?`, primaryLabel: "Save" };
}

export function removeSaveBar(document: Document): void {
  for (const existing of Array.from(document.querySelectorAll(hostTagName))) existing.remove();
}

export function showSaveBar(document: Document, offer: SaveOfferView, onAction: SaveBarActionHandler): SaveBar {
  removeSaveBar(document);
  const host = document.createElement(hostTagName);
  const shadowRoot = host.attachShadow({ mode: "closed" });
  const style = createElement(document, "style");
  style.textContent = styles;
  const bar = createElement(document, "div", "bar");
  bar.setAttribute("role", "dialog");
  bar.setAttribute("aria-label", "Hisar");
  const { title, primaryLabel } = describeSaveOffer(offer);

  const header = createElement(document, "div", "header");
  const logo = createElement(document, "span");
  logo.innerHTML = logoSvg;
  const heading = createElement(document, "p", "title", title);
  const closeButton = createElement(document, "button", "ghost close");
  closeButton.innerHTML = closeSvg;
  closeButton.setAttribute("aria-label", "Close");
  closeButton.dataset.action = "not-now";
  header.append(logo, heading, closeButton);

  const detail = createElement(document, "p", "detail");
  if (offer.username) {
    detail.append("Username ");
    detail.append(createElement(document, "strong", undefined, offer.username));
  } else {
    detail.textContent = "No username";
  }

  const message = createElement(document, "p", "message");
  message.hidden = true;

  const actions = createElement(document, "div", "actions");
  const neverButton = createElement(document, "button", "ghost", "Never for this site");
  neverButton.dataset.action = "never";
  const notNowButton = createElement(document, "button", undefined, "Not now");
  notNowButton.dataset.action = "not-now";
  const primaryButton = createElement(document, "button", "primary", primaryLabel);
  primaryButton.dataset.action = "save";
  actions.append(neverButton, notNowButton, primaryButton);

  bar.append(header, detail, message, actions);
  shadowRoot.append(style, bar);
  document.documentElement.append(host);

  const remove = (): void => host.remove();
  const allButton = [closeButton, neverButton, notNowButton, primaryButton];
  const act = async (action: SaveOfferAction): Promise<void> => {
    for (const button of allButton) button.disabled = true;
    const outcome = await onAction(action).catch((error: unknown) => ({
      ok: false,
      message: error instanceof Error ? error.message : String(error),
    }));
    if (action !== "save") {
      remove();
      return;
    }
    message.hidden = false;
    if (outcome.ok) {
      message.className = "message";
      message.textContent = offer.kind === "update" ? "Password updated in Hisar." : "Saved to Hisar.";
      actions.hidden = true;
      setTimeout(remove, 2500);
      return;
    }
    message.className = "message error";
    message.textContent = outcome.message || "Hisar could not save this login.";
    for (const button of allButton) button.disabled = false;
  };
  for (const button of allButton) {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      void act(button.dataset.action as SaveOfferAction);
    });
  }
  return { host, shadowRoot, remove };
}
