import { VisibilityGuard } from "../../shared/click-guard";
import { isExtensionPageUrl } from "../../shared/extension-url";
import { getParentAcrossShadow } from "../dom";
import { isRectInViewport, type Rect, type Viewport } from "../inline-menu/placement";

export const promptHostTagName = "hisar-passkey-prompt";
export const promptWidth = 360;
const promptMargin = 12;
const defaultPromptHeight = 180;

export interface PromptFrameDependencies {
  now: () => number;
  requestFrame: (callback: () => void) => number;
  cancelFrame: (handle: number) => void;
  getRect: (element: Element) => Rect;
  getElementAtPoint: (x: number, y: number) => Element | null;
  // extensionBaseUrl is chrome.runtime.getURL(""): the prompt opens only from this browser's extension pages.
  extensionBaseUrl: string;
}

const styles = `
:host { all: initial !important; position: fixed !important; inset: 0 auto auto 0 !important; width: 0 !important; height: 0 !important; z-index: 2147483647 !important; pointer-events: none !important; }
.prompt {
  position: fixed; border: none; margin: 0; padding: 0; border-radius: 12px; background: #111214; color-scheme: dark;
  box-shadow: 0 16px 40px rgb(0 0 0 / 45%); pointer-events: auto; opacity: 1; visibility: visible;
}
`;

export function computeTopRightRect(height: number, viewport: Viewport): Rect {
  const width = Math.min(promptWidth, viewport.width - promptMargin * 2);
  return { left: Math.max(viewport.width - width - promptMargin, promptMargin), top: promptMargin, width, height };
}

export class PasskeyPromptFrame {
  private host: HTMLElement | undefined;
  private iframe: HTMLIFrameElement | undefined;
  private height = defaultPromptHeight;
  private frameHandle: number | undefined;
  private readonly guard = new VisibilityGuard();

  constructor(
    private readonly document: Document,
    private readonly window: Window,
    private readonly dependencies: PromptFrameDependencies,
  ) {}

  isShown(): boolean {
    return this.iframe !== undefined;
  }

  getHost(): HTMLElement | undefined {
    return this.host;
  }

  getIframe(): HTMLIFrameElement | undefined {
    return this.iframe;
  }

  show(promptUrl: string): void {
    this.remove();
    if (!isExtensionPageUrl(promptUrl, this.dependencies.extensionBaseUrl)) return;
    const host = this.document.createElement(promptHostTagName);
    const shadowRoot = host.attachShadow({ mode: "closed" });
    const style = this.document.createElement("style");
    style.textContent = styles;
    const iframe = this.document.createElement("iframe");
    iframe.className = "prompt";
    iframe.src = promptUrl;
    iframe.setAttribute("title", "Hisar passkey");
    iframe.setAttribute("scrolling", "no");
    iframe.setAttribute("allowtransparency", "true");
    shadowRoot.append(style, iframe);
    (this.document.documentElement ?? this.document.body).append(host);
    this.host = host;
    this.iframe = iframe;
    this.height = defaultPromptHeight;
    this.guard.reset();
    this.updatePosition();
    this.scheduleFrame();
  }

  resize(height: number): void {
    if (!this.iframe) return;
    this.height = Math.max(60, Math.min(Math.round(height), 560));
    this.guard.reset();
    this.updatePosition();
  }

  isInteractionAllowed(): boolean {
    return this.iframe !== undefined && this.guard.isInteractionAllowed(this.dependencies.now());
  }

  remove(): void {
    if (this.frameHandle !== undefined) this.dependencies.cancelFrame(this.frameHandle);
    this.frameHandle = undefined;
    this.host?.remove();
    this.host = undefined;
    this.iframe = undefined;
    this.guard.reset();
  }

  private scheduleFrame(): void {
    this.frameHandle = this.dependencies.requestFrame(() => {
      this.frameHandle = undefined;
      if (!this.iframe) return;
      this.updatePosition();
      if (this.iframe) this.scheduleFrame();
    });
  }

  updatePosition(): void {
    const iframe = this.iframe;
    if (!iframe) return;
    const viewport = { width: this.window.innerWidth, height: this.window.innerHeight };
    const rect = computeTopRightRect(this.height, viewport);
    iframe.style.left = `${Math.round(rect.left)}px`;
    iframe.style.top = `${Math.round(rect.top)}px`;
    iframe.style.width = `${Math.round(rect.width)}px`;
    iframe.style.height = `${Math.round(rect.height)}px`;
    this.guard.recordVisibility(this.isUnobscured(viewport), this.dependencies.now());
  }

  private isUnobscured(viewport: Viewport): boolean {
    const iframe = this.iframe;
    if (!iframe || !this.host || this.document.visibilityState === "hidden") return false;
    const rect = this.dependencies.getRect(iframe);
    if (rect.width < 40 || rect.height < 30 || !isRectInViewport(rect, viewport)) return false;
    const view = this.document.defaultView;
    if (!view) return false;
    for (let element: Element | null = iframe; element; element = getParentAcrossShadow(element)) {
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
