import type { PopupView } from "../shared/messages";
import {
  passkeyContentPortName,
  passkeyPromptPath,
  passkeyPromptPortName,
  type BackgroundToContentPasskeyMessage,
  type ContentToBackgroundPasskeyMessage,
  type PasskeyChoice,
  type PasskeyCredentialJSON,
  type PasskeyPromptRequest,
  type PasskeyPromptResponse,
  type PasskeyPromptState,
} from "../shared/passkey";
import type { DesktopPasskeyParameters } from "../shared/passkey-protocol";
import { HisarError, toHisarError, type LoginSummary, type RequestMap } from "../shared/protocol";
import { readFrameOrigin, validatePasskeyRequest, type FrameOrigin, type PasskeySender, type ValidatedAssertion, type ValidatedCreation } from "./passkey-request";

export const visibilityCheckTimeoutMilliseconds = 1000;
export const allPasskeyTransport = ["hybrid", "internal"];

export interface PasskeyPort {
  name: string;
  sender?: PasskeySender;
  postMessage(message: unknown): void;
  disconnect(): void;
  onMessage: { addListener(listener: (message: unknown) => void): void };
  onDisconnect: { addListener(listener: () => void): void };
}

type PasskeyRequestType = "passkeys.list" | "passkeys.prepare" | "passkeys.create" | "passkeys.get";

export interface PasskeyDependencies {
  getDesktopView(): Promise<PopupView>;
  isDesktopAvailable(): Promise<boolean>;
  isDesktopConnected(): boolean;
  requestDesktop<Type extends PasskeyRequestType>(type: Type, passkey: DesktopPasskeyParameters): Promise<RequestMap[Type]["result"]>;
  unlock(): Promise<void>;
  getPromptUrl(): string;
  createID(): string;
}

interface PasskeySession {
  sessionID: string;
  tabId: number;
  frame: FrameOrigin;
  request: ValidatedCreation | ValidatedAssertion;
  contentPort: PasskeyPort;
  view: PasskeyPromptState["view"];
  allPasskey: PasskeyChoice[];
  allLogin: LoginSummary[];
  promptFrameId?: number;
  isFinished: boolean;
  allVisibilityCheck: Map<string, (isVisible: boolean) => void>;
}

function isPromptSender(sender: PasskeySender | undefined): boolean {
  try {
    const url = new URL(sender?.url ?? "");
    return url.protocol === "chrome-extension:" && url.pathname === passkeyPromptPath;
  } catch {
    return false;
  }
}

export class PasskeyService {
  private readonly allSession = new Map<string, PasskeySession>();

  constructor(private readonly dependencies: PasskeyDependencies) {}

  handlePort(port: PasskeyPort): void {
    if (port.name === passkeyContentPortName) this.handleContentPort(port);
    else if (port.name === passkeyPromptPortName) this.handlePromptPort(port);
  }

  getSessionCount(): number {
    return this.allSession.size;
  }

  private handleContentPort(port: PasskeyPort): void {
    let session: PasskeySession | undefined;
    let isStarted = false;
    let isEnded = false;
    const end = (): void => {
      isEnded = true;
      if (session) this.finishSession(session);
    };
    port.onDisconnect.addListener(end);
    port.onMessage.addListener((message) => {
      const contentMessage = message as ContentToBackgroundPasskeyMessage;
      switch (contentMessage?.type) {
        case "start":
          if (isStarted) return;
          isStarted = true;
          void this.start(port, contentMessage).then((started) => {
            session = started;
            if (isEnded && started) this.finishSession(started);
          });
          return;
        case "cancel":
          end();
          return;
        case "visibility":
          session?.allVisibilityCheck.get(contentMessage.checkID)?.(contentMessage.isVisible === true);
          return;
      }
    });
  }

  private post(port: PasskeyPort, message: BackgroundToContentPasskeyMessage): void {
    try {
      port.postMessage(message);
    } catch {
      // The page went away.
    }
  }

  private async start(port: PasskeyPort, message: Extract<ContentToBackgroundPasskeyMessage, { type: "start" }>): Promise<PasskeySession | undefined> {
    if (message.request?.kind === "availability") {
      this.post(port, { type: "availability", isAvailable: await this.dependencies.isDesktopAvailable().catch(() => false) });
      return undefined;
    }
    const tabId = port.sender?.tab?.id;
    const frame = port.sender ? readFrameOrigin(port.sender, message.allAncestorOrigin) : undefined;
    const validation = validatePasskeyRequest(message.request);
    if (tabId === undefined || !frame || (frame.isCrossOrigin && message.isPermissionPolicyAllowed !== true) || !validation.isValid) {
      this.post(port, { type: "fallback" });
      return undefined;
    }
    const request = validation.request;
    const isConditional = request.kind === "get" && request.isConditional;
    if (isConditional && !this.dependencies.isDesktopConnected()) {
      this.post(port, { type: "fallback" });
      return undefined;
    }
    const session: PasskeySession = {
      sessionID: this.dependencies.createID(),
      tabId,
      frame,
      request,
      contentPort: port,
      view: "locked",
      allPasskey: [],
      allLogin: [],
      isFinished: false,
      allVisibilityCheck: new Map(),
    };
    try {
      const desktopView = await this.dependencies.getDesktopView();
      if (desktopView !== "unlocked" && (desktopView !== "locked" || isConditional)) throw new HisarError("desktop-unavailable", desktopView);
      if (desktopView === "unlocked") await this.loadSession(session);
    } catch {
      this.post(port, { type: "fallback" });
      return undefined;
    }
    if (session.view === "no-passkeys") {
      this.post(port, { type: "fallback" });
      return undefined;
    }
    this.allSession.set(session.sessionID, session);
    this.post(port, { type: "prompt", promptUrl: `${this.dependencies.getPromptUrl()}#${session.sessionID}`, isConditional });
    return session;
  }

  private buildDesktopParameters(session: PasskeySession): DesktopPasskeyParameters {
    return {
      origin: session.frame.origin,
      topOrigin: session.frame.isCrossOrigin ? session.frame.topOrigin : undefined,
      isCrossOrigin: session.frame.isCrossOrigin,
      relyingPartyId: session.request.relyingPartyID,
    };
  }

  private async loadSession(session: PasskeySession): Promise<void> {
    const parameters = this.buildDesktopParameters(session);
    if (session.request.kind === "get") {
      const result = await this.dependencies.requestDesktop("passkeys.list", { ...parameters, allowCredentials: session.request.allAllowedCredentialID });
      session.allPasskey = (result.passkeys ?? []).map(({ credentialId, title, userName, userDisplayName }) => ({ credentialId, title, userName, userDisplayName }));
      session.view = session.allPasskey.length > 0 ? "choose" : "no-passkeys";
      return;
    }
    const result = await this.dependencies.requestDesktop("passkeys.prepare", { ...parameters, excludeCredentials: session.request.allExcludedCredentialID });
    session.allLogin = result.logins ?? [];
    session.view = result.isExcluded ? "excluded" : "save";
  }

  private finishSession(session: PasskeySession, message?: BackgroundToContentPasskeyMessage): void {
    if (session.isFinished) return;
    session.isFinished = true;
    this.allSession.delete(session.sessionID);
    for (const resolve of session.allVisibilityCheck.values()) resolve(false);
    session.allVisibilityCheck.clear();
    if (message) this.post(session.contentPort, message);
  }

  private handlePromptPort(port: PasskeyPort): void {
    if (!isPromptSender(port.sender) || port.sender?.tab?.id === undefined || port.sender.frameId === undefined) {
      port.disconnect();
      return;
    }
    const sender = { tabId: port.sender.tab.id, frameId: port.sender.frameId };
    let claimedSession: PasskeySession | undefined;
    port.onDisconnect.addListener(() => {
      if (claimedSession?.promptFrameId === sender.frameId) claimedSession.promptFrameId = undefined;
    });
    port.onMessage.addListener((message) => {
      const call = message as PasskeyPromptRequest;
      if (typeof call?.callID !== "string") return;
      void this.answerPromptCall(sender, call)
        .then((answer) => {
          claimedSession = answer.session;
          return { callID: call.callID, ok: true, result: answer.result ?? null } satisfies PasskeyPromptResponse;
        })
        .catch((error: unknown) => {
          const hisarError = toHisarError(error);
          return { callID: call.callID, ok: false, error: { code: hisarError.code, message: hisarError.message } } satisfies PasskeyPromptResponse;
        })
        .then((response) => this.post(port, response as never));
    });
  }

  private claimSession(sender: { tabId: number; frameId: number }, sessionID: unknown): PasskeySession {
    const session = typeof sessionID === "string" ? this.allSession.get(sessionID) : undefined;
    if (!session || session.isFinished || session.tabId !== sender.tabId) throw new HisarError("invalid-request", "This passkey request has ended");
    session.promptFrameId ??= sender.frameId;
    if (session.promptFrameId !== sender.frameId) throw new HisarError("invalid-request", "This passkey request has ended");
    return session;
  }

  private buildPromptState(session: PasskeySession): PasskeyPromptState {
    const relyingPartyID = session.request.relyingPartyID ?? new URL(session.frame.origin).hostname;
    return {
      view: session.view,
      kind: session.request.kind,
      relyingPartyID,
      userName: session.request.kind === "create" ? session.request.userName : "",
      allPasskey: session.allPasskey,
      allLogin: session.allLogin,
    };
  }

  private async answerPromptCall(sender: { tabId: number; frameId: number }, call: PasskeyPromptRequest): Promise<{ session: PasskeySession; result?: unknown }> {
    const session = this.claimSession(sender, call.sessionID);
    switch (call.method) {
      case "getState":
        if (session.view === "locked") {
          const desktopView = await this.dependencies.getDesktopView().catch(() => "desktop-unavailable" as const);
          if (desktopView === "unlocked") await this.loadSession(session);
        }
        return { session, result: this.buildPromptState(session) };
      case "unlock":
        await this.dependencies.unlock();
        return { session };
      case "choosePasskey":
        return { session, result: await this.choosePasskey(session, call.credentialId) };
      case "savePasskey":
        return { session, result: await this.savePasskey(session, call.vaultID, call.itemID) };
      case "confirmExcluded":
        if (session.view !== "excluded") throw new HisarError("invalid-request", "Nothing to confirm");
        this.finishSession(session, { type: "error", name: "InvalidStateError", message: "The authenticator already has a credential for this account" });
        return { session };
      case "useOtherDevice":
        this.finishSession(session, { type: "fallback" });
        return { session };
      case "dismiss":
        if (session.view === "locked" || session.view === "no-passkeys") this.finishSession(session, { type: "fallback" });
        else this.finishSession(session, { type: "error", name: "NotAllowedError", message: "The user closed the Hisar prompt" });
        return { session };
      case "resize":
        if (Number.isFinite(call.height)) this.post(session.contentPort, { type: "resize", height: call.height });
        return { session };
    }
    throw new HisarError("invalid-request", "Unknown passkey prompt call");
  }

  private async requireVisiblePrompt(session: PasskeySession): Promise<void> {
    const checkID = this.dependencies.createID();
    const isVisible = await new Promise<boolean>((resolve) => {
      const timer = setTimeout(() => resolve(false), visibilityCheckTimeoutMilliseconds);
      session.allVisibilityCheck.set(checkID, (result) => {
        clearTimeout(timer);
        resolve(result);
      });
      this.post(session.contentPort, { type: "checkVisibility", checkID });
    });
    session.allVisibilityCheck.delete(checkID);
    if (!isVisible) throw new HisarError("invalid-request", "The Hisar prompt was not fully visible. Try again");
  }

  private async requestForPrompt<Result>(session: PasskeySession, send: () => Promise<Result>): Promise<Result> {
    try {
      return await send();
    } catch (error) {
      const hisarError = toHisarError(error);
      if (hisarError.code === "locked") session.view = "locked";
      throw hisarError;
    }
  }

  private async choosePasskey(session: PasskeySession, credentialId: string): Promise<null> {
    const request = session.request;
    if (request.kind !== "get" || session.view !== "choose" || !session.allPasskey.some((choice) => choice.credentialId === credentialId)) {
      throw new HisarError("invalid-request", "Choose one of the passkeys shown");
    }
    await this.requireVisiblePrompt(session);
    const assertion = await this.requestForPrompt(session, () =>
      this.dependencies.requestDesktop("passkeys.get", {
        ...this.buildDesktopParameters(session),
        challenge: request.challenge,
        allowCredentials: request.allAllowedCredentialID,
        credentialId,
      }),
    );
    const credential: PasskeyCredentialJSON = {
      kind: "get",
      credentialId: assertion.credentialId,
      clientDataJSON: assertion.clientDataJSON,
      authenticatorData: assertion.authenticatorData,
      signature: assertion.signature,
      userHandle: assertion.userHandle,
    };
    this.finishSession(session, { type: "credential", credential });
    return null;
  }

  private async savePasskey(session: PasskeySession, vaultID: string | undefined, itemID: string | undefined): Promise<null> {
    const request = session.request;
    if (request.kind !== "create" || session.view !== "save") throw new HisarError("invalid-request", "There is no passkey to save");
    const isExistingLogin = vaultID !== undefined || itemID !== undefined;
    if (isExistingLogin && !session.allLogin.some((login) => login.vaultID === vaultID && login.itemID === itemID)) {
      throw new HisarError("invalid-request", "Choose one of the logins shown");
    }
    await this.requireVisiblePrompt(session);
    const creation = await this.requestForPrompt(session, () =>
      this.dependencies.requestDesktop("passkeys.create", {
        ...this.buildDesktopParameters(session),
        challenge: request.challenge,
        userHandle: request.userHandle,
        userName: request.userName,
        userDisplayName: request.userDisplayName,
        excludeCredentials: request.allExcludedCredentialID,
        ...(isExistingLogin ? { vaultID, itemID } : {}),
      }),
    );
    const credential: PasskeyCredentialJSON = {
      kind: "create",
      credentialId: creation.credentialId,
      clientDataJSON: creation.clientDataJSON,
      attestationObject: creation.attestationObject,
      authenticatorData: creation.authenticatorData,
      publicKey: creation.publicKey,
      publicKeyAlgorithm: creation.publicKeyAlgorithm,
      transports: allPasskeyTransport,
      hasCredPropsExtension: request.hasCredPropsExtension,
    };
    this.finishSession(session, { type: "credential", credential });
    return null;
  }
}
