import {
  passkeyCancelEventName,
  passkeyRequestEventName,
  passkeyResponseEventName,
  type BackgroundToContentPasskeyMessage,
  type ContentToBackgroundPasskeyMessage,
  type PasskeyPageRequest,
  type PasskeyPageResponse,
} from "../../shared/passkey";
import type { PasskeyPromptFrame } from "./prompt-frame";

export const keepAliveMilliseconds = 20_000;

export interface PasskeyContentPort {
  postMessage(message: ContentToBackgroundPasskeyMessage): void;
  disconnect(): void;
  onMessage: { addListener(listener: (message: BackgroundToContentPasskeyMessage) => void): void };
  onDisconnect: { addListener(listener: () => void): void };
}

export interface PasskeyContentDependencies {
  connect: () => PasskeyContentPort;
  createPromptFrame: () => PasskeyPromptFrame;
  getAllAncestorOrigin: () => string[];
  isPermissionPolicyAllowed: (feature: string) => boolean;
  setInterval: (callback: () => void, milliseconds: number) => ReturnType<typeof setInterval>;
  clearInterval: (handle: ReturnType<typeof setInterval>) => void;
}

interface ActiveRequest {
  requestID: string;
  port: PasskeyContentPort;
  keepAliveTimer?: ReturnType<typeof setInterval>;
  promptUrl?: string;
  isConditional: boolean;
  isSettled: boolean;
}

export function isWebAuthnField(target: EventTarget | null | undefined): target is HTMLInputElement {
  if (!target || (target as Element).tagName !== "INPUT") return false;
  const autocomplete = (target as HTMLInputElement).getAttribute("autocomplete") ?? "";
  return autocomplete.toLowerCase().split(/\s+/).includes("webauthn");
}

function parseRequest(detail: unknown): PasskeyPageRequest | undefined {
  try {
    const request = JSON.parse(String(detail)) as PasskeyPageRequest;
    if (typeof request?.requestID !== "string" || !["create", "get", "availability"].includes(request.kind)) return undefined;
    return request;
  } catch {
    return undefined;
  }
}

export class PasskeyContentBridge {
  private active: ActiveRequest | undefined;
  private readonly promptFrame: PasskeyPromptFrame;
  private readonly allDetach: (() => void)[] = [];

  constructor(
    private readonly document: Document,
    private readonly dependencies: PasskeyContentDependencies,
  ) {
    this.promptFrame = dependencies.createPromptFrame();
  }

  start(): void {
    this.listen(passkeyRequestEventName, (event) => this.handleRequest((event as CustomEvent).detail));
    this.listen(passkeyCancelEventName, (event) => this.handleCancel((event as CustomEvent).detail));
    this.listen("focusin", (event) => this.handleFocusIn(event), true);
    this.listen("focusout", () => this.handleFocusOut(), true);
  }

  stop(): void {
    for (const detach of this.allDetach.splice(0)) detach();
    if (this.active) this.settle(this.active, { requestID: this.active.requestID, outcome: "fallback" });
  }

  private listen(type: string, listener: (event: Event) => void, isCapture = false): void {
    this.document.addEventListener(type, listener, isCapture);
    this.allDetach.push(() => this.document.removeEventListener(type, listener, isCapture));
  }

  getPromptFrame(): PasskeyPromptFrame {
    return this.promptFrame;
  }

  private respond(response: PasskeyPageResponse): void {
    this.document.dispatchEvent(new CustomEvent(passkeyResponseEventName, { detail: JSON.stringify(response) }));
  }

  handleRequest(detail: unknown): void {
    const request = parseRequest(detail);
    if (!request) return;
    if (request.kind !== "availability") {
      this.respond({ requestID: request.requestID, outcome: "accepted" });
      if (this.active) this.settle(this.active, { requestID: this.active.requestID, outcome: "error", name: "NotAllowedError", message: "Another passkey request started" });
    }
    let port: PasskeyContentPort;
    try {
      port = this.dependencies.connect();
    } catch {
      this.respond(request.kind === "availability" ? { requestID: request.requestID, outcome: "availability", isAvailable: false } : { requestID: request.requestID, outcome: "fallback" });
      return;
    }
    const active: ActiveRequest = { requestID: request.requestID, port, isConditional: false, isSettled: false };
    if (request.kind !== "availability") {
      this.active = active;
      active.keepAliveTimer = this.dependencies.setInterval(() => this.post(active, { type: "keepAlive" }), keepAliveMilliseconds);
    }
    port.onMessage.addListener((message) => this.handlePortMessage(active, message));
    port.onDisconnect.addListener(() => {
      if (request.kind === "availability") this.settle(active, { requestID: request.requestID, outcome: "availability", isAvailable: false });
      else this.settle(active, { requestID: request.requestID, outcome: "fallback" });
    });
    const feature = request.kind === "create" ? "publickey-credentials-create" : "publickey-credentials-get";
    this.post(active, {
      type: "start",
      request,
      allAncestorOrigin: this.dependencies.getAllAncestorOrigin(),
      isPermissionPolicyAllowed: this.dependencies.isPermissionPolicyAllowed(feature),
    });
  }

  private post(active: ActiveRequest, message: ContentToBackgroundPasskeyMessage): void {
    try {
      active.port.postMessage(message);
    } catch {
      this.settle(active, { requestID: active.requestID, outcome: "fallback" });
    }
  }

  private handlePortMessage(active: ActiveRequest, message: BackgroundToContentPasskeyMessage): void {
    if (active.isSettled) return;
    switch (message?.type) {
      case "fallback":
        this.settle(active, { requestID: active.requestID, outcome: "fallback" });
        return;
      case "availability":
        this.settle(active, { requestID: active.requestID, outcome: "availability", isAvailable: message.isAvailable === true });
        return;
      case "credential":
        this.settle(active, { requestID: active.requestID, outcome: "credential", credential: message.credential });
        return;
      case "error":
        this.settle(active, { requestID: active.requestID, outcome: "error", name: String(message.name), message: String(message.message) });
        return;
      case "prompt":
        active.promptUrl = message.promptUrl;
        active.isConditional = message.isConditional === true;
        if (!active.isConditional || isWebAuthnField(this.findFocusedElement())) this.promptFrame.show(message.promptUrl);
        return;
      case "checkVisibility":
        this.post(active, { type: "visibility", checkID: message.checkID, isVisible: this.promptFrame.isInteractionAllowed() });
        return;
      case "resize":
        this.promptFrame.resize(message.height);
        return;
    }
  }

  private settle(active: ActiveRequest, response: PasskeyPageResponse): void {
    if (active.isSettled) return;
    active.isSettled = true;
    if (active.keepAliveTimer !== undefined) this.dependencies.clearInterval(active.keepAliveTimer);
    if (this.active === active) {
      this.active = undefined;
      this.promptFrame.remove();
    }
    try {
      active.port.disconnect();
    } catch {
      // Already disconnected.
    }
    this.respond(response);
  }

  private handleCancel(detail: unknown): void {
    let requestID: unknown;
    try {
      requestID = (JSON.parse(String(detail)) as { requestID?: unknown })?.requestID;
    } catch {
      return;
    }
    const active = this.active;
    if (!active || active.requestID !== requestID || active.isSettled) return;
    this.post(active, { type: "cancel" });
    active.isSettled = true;
    if (active.keepAliveTimer !== undefined) this.dependencies.clearInterval(active.keepAliveTimer);
    this.active = undefined;
    this.promptFrame.remove();
    try {
      active.port.disconnect();
    } catch {
      // Already disconnected.
    }
  }

  private findFocusedElement(): Element | null {
    let active: Element | null = this.document.activeElement;
    while (active?.shadowRoot?.activeElement) active = active.shadowRoot.activeElement;
    return active;
  }

  private handleFocusIn(event: Event): void {
    const active = this.active;
    if (!active?.isConditional || !active.promptUrl) return;
    const target = event.composedPath()[0];
    if (this.promptFrame.getHost() && target === this.promptFrame.getHost()) return;
    if (isWebAuthnField(target) && !this.promptFrame.isShown()) this.promptFrame.show(active.promptUrl);
  }

  private handleFocusOut(): void {
    if (!this.active?.isConditional || !this.promptFrame.isShown()) return;
    setTimeout(() => {
      const focused = this.document.activeElement;
      if (focused === this.promptFrame.getHost()) return;
      if (!isWebAuthnField(this.findFocusedElement())) this.promptFrame.remove();
    }, 0);
  }
}
