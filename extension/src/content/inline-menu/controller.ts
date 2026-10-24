import { VisibilityGuard } from "../../shared/click-guard";
import { isExtensionPageUrl } from "../../shared/extension-url";
import type { ContentToBackgroundMessage, InlineMenuKey, InlineMenuPurpose, MessageResponse } from "../../shared/messages";
import { defaultPageSettings, type PageSettings } from "../../shared/settings";
import { getDeepActiveElement, getParentAcrossShadow } from "../dom";
import { findOneTimeCodeTarget } from "../one-time-code";
import { findInlineMenuPurpose, isFrameEligibleForInlineMenu } from "./eligibility";
import { computeIconRect, computeIconRectBeside, computeMenuRect, computeUnionRect, iconSize, isRectInViewport, type Rect } from "./placement";

export interface InlineMenuDependencies {
  sendToBackground: <Result>(message: ContentToBackgroundMessage) => Promise<MessageResponse<Result>>;
  now: () => number;
  requestFrame: (callback: () => void) => number;
  cancelFrame: (handle: number) => void;
  getRect: (element: Element) => Rect;
  getElementAtPoint: (x: number, y: number) => Element | null;
  isTrustedEvent?: (event: Event) => boolean;
  // extensionBaseUrl is chrome.runtime.getURL(""): the menu opens only from this browser's extension pages.
  extensionBaseUrl: string;
}

const hostTagName = "hisar-inline-menu";
const defaultMenuHeight = 120;
const allMenuKey: InlineMenuKey[] = ["ArrowDown", "ArrowUp", "Enter"];

const styles = `
:host { all: initial !important; position: fixed !important; inset: 0 auto auto 0 !important; width: 0 !important; height: 0 !important; z-index: 2147483647 !important; pointer-events: none !important; }
.icon {
  position: fixed; box-sizing: border-box; width: ${iconSize}px; height: ${iconSize}px; padding: 0; margin: 0;
  border: none; border-radius: 5px; background: transparent; cursor: pointer; pointer-events: auto;
  display: flex; align-items: center; justify-content: center; opacity: 0.92;
}
.icon:hover { opacity: 1; box-shadow: 0 0 0 2px rgb(138 149 165 / 35%); }
.menu {
  position: fixed; border: none; margin: 0; padding: 0; border-radius: 10px; background: #111214; color-scheme: dark;
  box-shadow: 0 12px 32px rgb(0 0 0 / 45%); pointer-events: auto; opacity: 1; visibility: visible;
}
`;

const logoSvg = `<svg width="${iconSize}" height="${iconSize}" viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="8" fill="#16181b"/><rect x="1" y="1" width="30" height="30" rx="7" fill="none" stroke="#30343a" stroke-width="2"/><path d="M8 24.5V10.5h3V7.5h2.5v3h5V7.5H21v3h3v14h-6v-5a2 2 0 0 0-4 0v5z" fill="#eceef1"/></svg>`;

interface OpenMenu {
  sessionID: string;
  iframe: HTMLIFrameElement;
  height: number;
  hasKeyboardSelection: boolean;
}

export class InlineMenuController {
  private settings: PageSettings = { ...defaultPageSettings };
  // isAddressFillAvailable: the paired app fills addresses; an older one does not, so address fields get no icon.
  private isAddressFillAvailable = false;
  // isCardFillAvailable: the paired app fills cards; with an older one, payment fields get no icon.
  private isCardFillAvailable = false;
  // isIdentityFillAvailable: the paired app fills identities; with an older one, name and email fields get no icon.
  private isIdentityFillAvailable = false;
  private field: HTMLInputElement | undefined;
  private allGroupField: HTMLInputElement[] = [];
  private purpose: InlineMenuPurpose | undefined;
  private host: HTMLElement | undefined;
  private shadowRoot: ShadowRoot | undefined;
  private icon: HTMLButtonElement | undefined;
  private menu: OpenMenu | undefined;
  private isOpening = false;
  private isFilling = false;
  private trustedInputCount = 0;
  private frameHandle: number | undefined;
  private readonly guard = new VisibilityGuard();
  private readonly allDetach: (() => void)[] = [];

  constructor(
    private readonly document: Document,
    private readonly window: Window,
    private readonly dependencies: InlineMenuDependencies,
  ) {}

  private isTrusted(event: Event): boolean {
    return (this.dependencies.isTrustedEvent ?? ((candidate: Event) => candidate.isTrusted))(event);
  }

  start(): void {
    if (!isFrameEligibleForInlineMenu(this.window)) return;
    this.listen(this.document, "focusin", (event) => this.handleFocusIn(event), true);
    this.listen(this.document, "focusout", () => this.handleFocusOut(), true);
    this.listen(this.document, "keydown", (event) => this.handleKeydown(event as KeyboardEvent), true);
    this.listen(this.document, "pointerdown", (event) => this.handlePointerDown(event), true);
    this.listen(this.document, "input", (event) => this.handleInput(event), true);
    this.listen(this.window, "pagehide", () => this.hide());
    // A field focused before the script started, as a dialog or the page focuses its first field, gets no focusin.
    const active = getDeepActiveElement(this.document);
    if (active && active !== this.document.body) this.focusOn(active);
  }

  stop(): void {
    this.hide();
    for (const detach of this.allDetach.splice(0)) detach();
  }

  updateSettings(settings: PageSettings): void {
    this.settings = settings;
    if (!settings.isInlineMenuEnabled) this.hide();
  }

  setAddressFillAvailable(isAvailable: boolean): void {
    this.isAddressFillAvailable = isAvailable;
    if (!isAvailable && this.purpose === "address") this.hide();
  }

  setIdentityFillAvailable(isAvailable: boolean): void {
    this.isIdentityFillAvailable = isAvailable;
    if (!isAvailable && this.purpose === "identity") this.hide();
  }

  setCardFillAvailable(isAvailable: boolean): void {
    this.isCardFillAvailable = isAvailable;
    if (!isAvailable && this.purpose === "card") this.hide();
  }

  // runFill runs a fill. The focus changes it makes leave the menu where it is, so a failed fill reports in the menu;
  // the menu of sessionID closes only after a fill that wrote a value, and the icon then moves to the focused field
  // without opening the menu there.
  async runFill<Result>(sessionID: string | undefined, fill: () => Result | Promise<Result>, isFilled: (result: Result) => boolean): Promise<Result> {
    this.isFilling = true;
    let result: Result;
    try {
      result = await fill();
    } finally {
      this.isFilling = false;
    }
    if (sessionID !== undefined && !isFilled(result)) return result;
    if (sessionID !== undefined) this.dismissMenu(sessionID);
    const active = getDeepActiveElement(this.document);
    if (active && active !== this.field && !(this.host && this.host.contains(active))) this.anchorTo(active);
    return result;
  }

  getAnchoredField(sessionID: string): HTMLInputElement | undefined {
    return this.menu?.sessionID === sessionID ? this.field : undefined;
  }

  isMenuInteractionAllowed(sessionID: string): boolean {
    return this.menu?.sessionID === sessionID && this.guard.isInteractionAllowed(this.dependencies.now());
  }

  resizeMenu(sessionID: string, height: number): void {
    if (this.menu?.sessionID !== sessionID) return;
    this.menu.height = Math.max(40, Math.min(Math.round(height), 480));
    this.guard.reset();
    this.updatePosition();
  }

  dismissMenu(sessionID: string): void {
    if (this.menu?.sessionID === sessionID) this.closeMenu();
  }

  getIconElement(): HTMLButtonElement | undefined {
    return this.icon;
  }

  getMenuElement(): HTMLIFrameElement | undefined {
    return this.menu?.iframe;
  }

  async openMenu(): Promise<void> {
    if (this.menu || this.isOpening || !this.field || !this.purpose) return;
    this.isOpening = true;
    const field = this.field;
    const inputCountAtRequest = this.trustedInputCount;
    try {
      const response = await this.dependencies.sendToBackground<{ sessionID: string; menuUrl: string }>({ type: "content.openInlineMenu", purpose: this.purpose });
      if (!response.ok || this.field !== field || !this.shadowRoot || this.trustedInputCount !== inputCountAtRequest) {
        if (response.ok) void this.dependencies.sendToBackground({ type: "content.closeInlineMenu", sessionID: response.result.sessionID }).catch(() => undefined);
        return;
      }
      const iframe = this.document.createElement("iframe");
      iframe.className = "menu";
      if (!isExtensionPageUrl(response.result.menuUrl, this.dependencies.extensionBaseUrl)) return;
      iframe.src = response.result.menuUrl;
      iframe.setAttribute("title", "Hisar");
      iframe.setAttribute("scrolling", "no");
      iframe.setAttribute("allowtransparency", "true");
      this.shadowRoot.append(iframe);
      this.menu = { sessionID: response.result.sessionID, iframe, height: defaultMenuHeight, hasKeyboardSelection: false };
      this.guard.reset();
      this.updatePosition();
    } finally {
      this.isOpening = false;
    }
  }

  closeMenu(): void {
    const menu = this.menu;
    if (!menu) return;
    this.menu = undefined;
    this.guard.reset();
    menu.iframe.remove();
    void this.dependencies.sendToBackground({ type: "content.closeInlineMenu", sessionID: menu.sessionID }).catch(() => undefined);
  }

  hide(): void {
    this.closeMenu();
    this.field = undefined;
    this.allGroupField = [];
    this.purpose = undefined;
    if (this.frameHandle !== undefined) this.dependencies.cancelFrame(this.frameHandle);
    this.frameHandle = undefined;
    this.host?.remove();
    this.host = undefined;
    this.shadowRoot = undefined;
    this.icon = undefined;
  }

  private listen(target: EventTarget, type: string, listener: (event: Event) => void, isCapture = false): void {
    target.addEventListener(type, listener, isCapture);
    this.allDetach.push(() => target.removeEventListener(type, listener, isCapture));
  }

  private handleFocusIn(event: Event): void {
    const target = event.composedPath()[0];
    if (target instanceof Element) this.focusOn(target);
  }

  private focusOn(target: Element, isRetry = false): void {
    if (!this.settings.isInlineMenuEnabled || this.isFilling) return;
    // A field read-only until focused, a trick against autofill, becomes editable in its own focus handler.
    if (!isRetry && (target as HTMLInputElement).readOnly === true) {
      setTimeout(() => {
        if (getDeepActiveElement(this.document) === target) this.focusOn(target, true);
      }, 0);
      return;
    }
    if (this.host && (target === this.host || this.host.contains(target))) return;
    if (this.isFieldOfMenu(target)) return;
    if (this.anchorTo(target) && this.settings.isInlineMenuOpenedOnFocus) void this.openMenu();
  }

  // anchorTo shows the icon in a field the menu serves and reports whether it is one; any other field hides it.
  private anchorTo(target: Element): boolean {
    const purpose = findInlineMenuPurpose(this.document, target, this.dependencies.getRect);
    if (!purpose || (purpose === "address" && !this.isAddressFillAvailable) || (purpose === "card" && !this.isCardFillAvailable) || (purpose === "identity" && !this.isIdentityFillAvailable)) {
      this.hide();
      return false;
    }
    this.hide();
    this.field = target as HTMLInputElement;
    this.allGroupField = purpose === "one-time-code" ? (findOneTimeCodeTarget(target) ?? [this.field]) : [this.field];
    this.purpose = purpose;
    this.showIcon();
    return true;
  }

  // openHere opens the menu at the focused field, for the right-click menu, even where it does not open on focus; it
  // reports whether that field is one the menu serves.
  openHere(): boolean {
    if (!isFrameEligibleForInlineMenu(this.window)) return false;
    const active = getDeepActiveElement(this.document);
    if (!active || !this.anchorTo(active)) return false;
    void this.openMenu();
    return true;
  }

  private handleFocusOut(): void {
    setTimeout(() => {
      if (!this.field || this.isFilling) return;
      const active = this.document.activeElement;
      if (active === this.host) return;
      let deepActive: Element | null = active;
      while (deepActive?.shadowRoot?.activeElement) deepActive = deepActive.shadowRoot.activeElement;
      if (!deepActive || !this.isFieldOfMenu(deepActive)) this.hide();
    }, 0);
  }

  private handleKeydown(event: KeyboardEvent): void {
    const target = event.composedPath()[0];
    if (!this.field || !this.isTrusted(event) || !(target instanceof Element) || !this.isFieldOfMenu(target)) return;
    if (event.key === "Escape" && this.menu) {
      event.preventDefault();
      this.closeMenu();
      return;
    }
    if (!allMenuKey.includes(event.key as InlineMenuKey)) return;
    if (!this.menu) {
      if (event.key === "ArrowDown" && this.settings.isInlineMenuEnabled) {
        event.preventDefault();
        void this.openMenu();
      }
      return;
    }
    if (event.key === "Enter" && !this.menu.hasKeyboardSelection) return;
    if (event.key !== "Enter") this.menu.hasKeyboardSelection = true;
    event.preventDefault();
    event.stopPropagation();
    void this.dependencies
      .sendToBackground({ type: "content.pressInlineMenuKey", sessionID: this.menu.sessionID, key: event.key as InlineMenuKey })
      .catch(() => undefined);
  }

  private handleInput(event: Event): void {
    const target = event.composedPath()[0];
    if (!this.isTrusted(event) || !(target instanceof Element) || !this.isFieldOfMenu(target)) return;
    this.trustedInputCount += 1;
    this.closeMenu();
  }

  private handlePointerDown(event: Event): void {
    const target = event.composedPath()[0];
    if (!this.host || !(target instanceof Node)) return;
    if (target === this.host || this.host.contains(target) || (target instanceof Element && this.isFieldOfMenu(target))) return;
    this.closeMenu();
  }

  private isFieldOfMenu(element: Element): boolean {
    return element === this.field || this.allGroupField.includes(element as HTMLInputElement);
  }

  private getFieldRect(): Rect {
    if (this.allGroupField.length <= 1 || !this.field) return this.dependencies.getRect(this.field as Element);
    return computeUnionRect(this.allGroupField.map((field) => this.dependencies.getRect(field)));
  }

  private showIcon(): void {
    const host = this.document.createElement(hostTagName);
    const shadowRoot = host.attachShadow({ mode: "closed" });
    const style = this.document.createElement("style");
    style.textContent = styles;
    const icon = this.document.createElement("button");
    icon.className = "icon";
    icon.type = "button";
    icon.tabIndex = -1;
    icon.title = "Hisar";
    icon.setAttribute("aria-label", "Open Hisar");
    icon.innerHTML = logoSvg;
    icon.addEventListener("mousedown", (event) => event.preventDefault());
    icon.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      if (!this.isTrusted(event)) return;
      if (this.menu) this.closeMenu();
      else void this.openMenu();
    });
    shadowRoot.append(style, icon);
    // In the top layer the host is drawn above the page and outside any filter or transform on the page's elements. Inside
    // the field's modal dialog or popover it is not inert, as the rest of the page is while a modal dialog is open.
    const isPopoverSupported = typeof host.showPopover === "function";
    if (isPopoverSupported) host.setAttribute("popover", "manual");
    (findTopLayerAncestor(this.field) ?? this.document.documentElement).append(host);
    if (isPopoverSupported) host.showPopover();
    this.host = host;
    this.shadowRoot = shadowRoot;
    this.icon = icon;
    this.updatePosition();
    this.scheduleFrame();
  }

  private scheduleFrame(): void {
    this.frameHandle = this.dependencies.requestFrame(() => {
      this.frameHandle = undefined;
      if (!this.field) return;
      this.updatePosition();
      if (this.field) this.scheduleFrame();
    });
  }

  updatePosition(): void {
    const field = this.field;
    if (!field || !this.icon) return;
    const viewport = { width: this.window.innerWidth, height: this.window.innerHeight };
    const fieldRect = this.getFieldRect();
    const hasLayout = this.document.documentElement.getClientRects().length > 0;
    if (!field.isConnected || (hasLayout && !isRectInViewport(fieldRect, viewport))) {
      this.hide();
      return;
    }
    placeElement(this.icon, this.allGroupField.length > 1 ? computeIconRectBeside(fieldRect) : computeIconRect(fieldRect));
    if (!this.menu) return;
    placeElement(this.menu.iframe, computeMenuRect(fieldRect, this.menu.height, viewport));
    this.guard.recordVisibility(this.isMenuUnobscured(viewport), this.dependencies.now());
  }

  private isMenuUnobscured(viewport: { width: number; height: number }): boolean {
    if (!this.menu || !this.host) return false;
    if (this.document.visibilityState === "hidden") return false;
    const iframe = this.menu.iframe;
    const rect = this.dependencies.getRect(iframe);
    if (rect.width < 40 || rect.height < 30 || !isRectInViewport(rect, viewport)) return false;
    const view = this.document.defaultView;
    if (!view) return false;
    for (let element: Element | null = iframe; element; element = element === this.host && isInTopLayer(element) ? null : getParentAcrossShadow(element)) {
      const style = view.getComputedStyle(element);
      if (style.display === "none" || style.visibility === "hidden" || Number.parseFloat(style.opacity || "1") < 1) return false;
      if (style.filter && style.filter !== "none") return false;
    }
    const allPoint: [number, number][] = [
      [rect.left + rect.width / 2, rect.top + rect.height / 2],
      [rect.left + 8, rect.top + 8],
      [rect.left + rect.width - 8, rect.top + 8],
      [rect.left + 8, rect.top + rect.height - 8],
      [rect.left + rect.width - 8, rect.top + rect.height - 8],
    ];
    return allPoint.every(([x, y]) => this.dependencies.getElementAtPoint(x, y) === this.host);
  }
}

function isInTopLayer(element: Element): boolean {
  try {
    return element.matches(":popover-open") || element.matches("dialog:modal");
  } catch {
    return false;
  }
}

function findTopLayerAncestor(field: Element | undefined): Element | undefined {
  for (let element = field ? getParentAcrossShadow(field) : null; element; element = getParentAcrossShadow(element)) {
    if (isInTopLayer(element)) return element;
  }
  return undefined;
}

function placeElement(element: HTMLElement, rect: Rect): void {
  element.style.left = `${Math.round(rect.left)}px`;
  element.style.top = `${Math.round(rect.top)}px`;
  element.style.width = `${Math.round(rect.width)}px`;
  element.style.height = `${Math.round(rect.height)}px`;
}
