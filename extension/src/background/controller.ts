import type {
  BackgroundToContentMessage,
  InlineMenuBroadcast,
  FillOutcome,
  FrameProbe,
  MatchResult,
  PopupState,
  SaveOfferAction,
  SaveOfferView,
} from "../shared/messages";
import {
  HisarError,
  toHisarError,
  type Credentials,
  type GenerateOptions,
  type HelloResult,
  type LoginSummary,
  type OneTimeCode,
  type UpdateRequirement,
} from "../shared/protocol";
import { getHost, getOrigin, isWebUrl } from "../shared/url";
import { chooseFocusedFrame, chooseLoginFrame, chooseOneTimeCodeFrame, selectAllFillableFrame, type FrameInfo, type ProbedFrame } from "./frames";
import type { NativeClient } from "./native-client";
import { decideSaveOffer, RememberedUsernameStore, SaveOfferStore, toSaveOfferView } from "./save";
import type { ExtensionStorage } from "./storage";

export interface BrowserBridge {
  getTabUrl(tabId: number): Promise<string | undefined>;
  getAllFrameInfo(tabId: number): Promise<FrameInfo[]>;
  injectContentScript(tabId: number): Promise<void>;
  sendToFrame<Result>(tabId: number, frameId: number, message: BackgroundToContentMessage): Promise<Result>;
  broadcastToInlineMenu(message: InlineMenuBroadcast): Promise<void>;
  openPopup(tabId: number): Promise<void>;
  writeToClipboard(text: string): Promise<void>;
  getInlineMenuUrl(): string;
  getBrowserName(): string;
  getExtensionVersion(): string;
  createID(): string;
  now(): number;
}

export class BackgroundController {
  readonly allSaveOffer: SaveOfferStore;
  readonly allRememberedUsername: RememberedUsernameStore;
  private updateRequirement: UpdateRequirement | undefined;

  constructor(
    private readonly client: NativeClient,
    private readonly storage: ExtensionStorage,
    private readonly browser: BrowserBridge,
  ) {
    this.allSaveOffer = new SaveOfferStore(() => browser.now());
    this.allRememberedUsername = new RememberedUsernameStore(() => browser.now());
  }

  async handleHello(result: HelloResult): Promise<void> {
    this.updateRequirement = result.updateRequired;
    if (!result.isPaired) await this.storage.removeToken();
  }

  async getPopupState(shouldReconnect = false): Promise<PopupState> {
    let hello: HelloResult;
    try {
      const hasToken = (await this.storage.getToken()) !== undefined;
      hello = shouldReconnect || !hasToken ? await this.client.reconnect() : await this.client.connect();
    } catch (error) {
      return { view: "desktop-unavailable", errorMessage: toHisarError(error).message };
    }
    if (hello.updateRequired) return { view: "update-required", updateRequired: hello.updateRequired };
    if (!(await this.storage.getToken())) return { view: hello.state === "signed-out" ? "signed-out" : "not-paired" };
    try {
      const status = await this.client.request("status", {});
      return { view: status.state, email: status.email, isOffline: status.isOffline };
    } catch (error) {
      const hisarError = toHisarError(error);
      if (hisarError.code === "not-paired") {
        await this.storage.removeToken();
        return { view: "not-paired" };
      }
      if (hisarError.code === "signed-out" || hisarError.code === "locked") return { view: hisarError.code };
      if (hisarError.code === "desktop-unavailable") return { view: "desktop-unavailable", errorMessage: hisarError.message };
      if (hisarError.code === "update-required") return { view: "update-required", errorMessage: hisarError.message };
      throw hisarError;
    }
  }

  async startPairing(): Promise<void> {
    const parameters = await this.getHelloParameters();
    await this.client.request("pair.start", { instanceID: parameters.instanceID, browser: parameters.browser });
  }

  async finishPairing(code: string): Promise<void> {
    const normalizedCode = code.replace(/\D/g, "");
    if (normalizedCode.length !== 6) throw new HisarError("invalid-request", "Enter the 6-digit code shown in Hisar");
    const instanceID = await this.storage.getInstanceID();
    const result = await this.request(() => this.client.request("pair.finish", { instanceID, code: normalizedCode }));
    if (!result.token) throw new HisarError("pairing-failed", "Hisar did not return a pairing token");
    await this.storage.setToken(result.token);
  }

  async unlock(): Promise<void> {
    await this.request(() => this.client.request("unlock", {}));
  }

  async getTabUrl(tabId: number): Promise<string | undefined> {
    return this.browser.getTabUrl(tabId);
  }

  async matchLoginsForUrl(url: string): Promise<LoginSummary[]> {
    if (!isWebUrl(url)) return [];
    const result = await this.request(() => this.client.request("logins.match", { url }));
    return result.logins ?? [];
  }

  async getCredentials(vaultID: string, itemID: string, url: string): Promise<Credentials> {
    return this.request(() => this.client.request("logins.fill", { vaultID, itemID, url }));
  }

  async matchLogins(tabId: number): Promise<MatchResult> {
    const url = await this.browser.getTabUrl(tabId);
    if (!isWebUrl(url)) return { url: url ?? "", allLogin: [] };
    const result = await this.request(() => this.client.request("logins.match", { url }));
    return { url, allLogin: result.logins ?? [] };
  }

  async getOneTimeCode(vaultID: string, itemID: string, url: string): Promise<OneTimeCode> {
    return this.request(() => this.client.request("logins.code", { vaultID, itemID, url }));
  }

  async getOneTimeCodeForTab(tabId: number, vaultID: string, itemID: string): Promise<OneTimeCode> {
    const url = await this.browser.getTabUrl(tabId);
    if (!isWebUrl(url)) throw new HisarError("no-match", "This page has no web address");
    return this.getOneTimeCode(vaultID, itemID, url);
  }

  // copyOneTimeCodeAfterFill puts the code on the clipboard for the next step of the sign-in; the fill has already succeeded.
  async copyOneTimeCodeAfterFill(credentials: Credentials, vaultID: string, itemID: string, url: string): Promise<boolean> {
    if (!credentials.hasOneTimePassword) return false;
    try {
      const { code } = await this.getOneTimeCode(vaultID, itemID, url);
      await this.browser.writeToClipboard(code);
      return true;
    } catch {
      return false;
    }
  }

  async fillLogin(tabId: number, vaultID: string, itemID: string): Promise<FillOutcome> {
    const allProbed = await this.probeFillableFrames(tabId);
    const target = chooseLoginFrame(allProbed);
    if (!target) {
      const codeTarget = chooseOneTimeCodeFrame(allProbed);
      if (codeTarget) return this.fillOneTimeCodeIntoFrame(tabId, codeTarget, vaultID, itemID);
      throw new HisarError("not-found", "No login form found on this page");
    }
    const credentials = await this.request(() => this.client.request("logins.fill", { vaultID, itemID, url: target.url }));
    const outcome = await this.browser.sendToFrame<FillOutcome>(tabId, target.frameId, {
      type: "content.fillLogin",
      expectedOrigin: target.origin,
      username: credentials.username,
      password: credentials.password,
    });
    if (!outcome?.filledUsername && !outcome?.filledPassword) throw new HisarError("not-found", "No login form found on this page");
    await this.copyOneTimeCodeAfterFill(credentials, vaultID, itemID, target.url);
    return outcome;
  }

  private async fillOneTimeCodeIntoFrame(tabId: number, target: FrameInfo, vaultID: string, itemID: string): Promise<FillOutcome> {
    const { code } = await this.getOneTimeCode(vaultID, itemID, target.url).catch((error: unknown) => {
      const hisarError = toHisarError(error);
      throw hisarError.code === "not-found" ? new HisarError("not-found", "This login has no one-time password") : hisarError;
    });
    const isFilled = await this.browser.sendToFrame<boolean>(tabId, target.frameId, { type: "content.fillOneTimeCode", expectedOrigin: target.origin, code });
    if (!isFilled) throw new HisarError("not-found", "No one-time code field found on this page");
    return { filledUsername: false, filledPassword: false, filledOneTimeCode: true };
  }

  async fillFirstLogin(tabId: number): Promise<FillOutcome> {
    const { allLogin } = await this.matchLogins(tabId);
    const firstLogin = allLogin[0];
    if (!firstLogin) throw new HisarError("no-match", "No saved login matches this page");
    return this.fillLogin(tabId, firstLogin.vaultID, firstLogin.itemID);
  }

  async getPasswordForTab(tabId: number, vaultID: string, itemID: string): Promise<string> {
    const url = await this.browser.getTabUrl(tabId);
    if (!isWebUrl(url)) throw new HisarError("no-match", "This page has no web address");
    const credentials = await this.request(() => this.client.request("logins.fill", { vaultID, itemID, url }));
    return credentials.password;
  }

  async generatePassword(options: GenerateOptions): Promise<string> {
    const result = await this.request(() => this.client.request("password.generate", options));
    return result.password;
  }

  async fillPassword(tabId: number, password: string): Promise<void> {
    const allProbed = await this.probeFillableFrames(tabId);
    const target = chooseFocusedFrame(allProbed);
    if (!target) throw new HisarError("not-found", "Click into a field on the page first");
    const isFilled = await this.browser.sendToFrame<boolean>(tabId, target.frameId, {
      type: "content.fillPassword",
      expectedOrigin: target.origin,
      password,
    });
    if (!isFilled) throw new HisarError("not-found", "Click into a field on the page first");
  }

  handleUsernameSubmitted(tabId: number, frameUrl: string, username: string): void {
    if (!isWebUrl(frameUrl) || !username) return;
    this.allRememberedUsername.remember(tabId, getOrigin(frameUrl), username);
  }

  async handleLoginSubmitted(tabId: number, frameUrl: string, username: string, password: string): Promise<SaveOfferView | undefined> {
    if (!isWebUrl(frameUrl) || !password) return undefined;
    const origin = getOrigin(frameUrl);
    if (await this.storage.isNeverOrigin(origin)) return undefined;
    if (!(await this.storage.getToken())) return undefined;
    const effectiveUsername = username || this.allRememberedUsername.recall(tabId, origin) || "";
    let checkResult;
    try {
      checkResult = await this.request(() =>
        this.client.request("logins.check", { url: frameUrl, username: effectiveUsername, password }),
      );
    } catch {
      return undefined;
    }
    const kind = decideSaveOffer(checkResult, false);
    if (!kind) return undefined;
    const offer = {
      offerID: this.browser.createID(),
      tabId,
      url: frameUrl,
      host: getHost(frameUrl),
      username: effectiveUsername,
      password,
      kind,
      vaultID: kind === "update" ? checkResult.vaultID : undefined,
      itemID: kind === "update" ? checkResult.itemID : undefined,
      title: checkResult.title,
      createdAt: this.browser.now(),
    };
    this.allSaveOffer.put(offer);
    const view = toSaveOfferView(offer);
    await this.browser.sendToFrame(tabId, 0, { type: "content.showSaveOffer", offer: view }).catch(() => undefined);
    return view;
  }

  getPendingSaveOffer(tabId: number): SaveOfferView | null {
    const offer = this.allSaveOffer.getForTab(tabId);
    return offer ? toSaveOfferView(offer) : null;
  }

  async actOnSaveOffer(tabId: number, offerID: string, action: SaveOfferAction): Promise<void> {
    const offer = this.allSaveOffer.getForTab(tabId);
    if (!offer || offer.offerID !== offerID) throw new HisarError("not-found", "This save offer has expired");
    if (action === "not-now") {
      this.allSaveOffer.take(tabId, offerID);
      return;
    }
    if (action === "never") {
      this.allSaveOffer.take(tabId, offerID);
      await this.storage.addNeverOrigin(getOrigin(offer.url));
      return;
    }
    const parameters =
      offer.kind === "update" && offer.vaultID && offer.itemID
        ? { url: offer.url, username: offer.username, password: offer.password, vaultID: offer.vaultID, itemID: offer.itemID }
        : { url: offer.url, username: offer.username, password: offer.password };
    await this.request(() => this.client.request("logins.save", parameters));
    this.allSaveOffer.take(tabId, offerID);
  }

  handleTabRemoved(tabId: number): void {
    this.allSaveOffer.removeTab(tabId);
    this.allRememberedUsername.removeTab(tabId);
  }

  async getHelloParameters(): Promise<{ instanceID: string; browser: string; extensionVersion: string }> {
    return {
      instanceID: await this.storage.getInstanceID(),
      browser: this.browser.getBrowserName(),
      extensionVersion: this.browser.getExtensionVersion(),
    };
  }

  private async request<Result>(send: () => Promise<Result>): Promise<Result> {
    if (this.updateRequirement?.target === "extension") {
      throw new HisarError("update-required", `Update the Hisar extension: this server requires version ${this.updateRequirement.minimumVersion} or later.`);
    }
    try {
      return await send();
    } catch (error) {
      const hisarError = toHisarError(error);
      if (hisarError.code === "not-paired") await this.storage.removeToken();
      throw hisarError;
    }
  }

  private async probeFillableFrames(tabId: number): Promise<ProbedFrame[]> {
    const allFillable = selectAllFillableFrame(await this.browser.getAllFrameInfo(tabId));
    if (allFillable.length === 0) throw new HisarError("not-found", "Hisar cannot fill this page");
    let allProbed = await this.probeFrames(tabId, allFillable);
    if (allProbed.length === 0) {
      await this.browser.injectContentScript(tabId);
      allProbed = await this.probeFrames(tabId, allFillable);
    }
    return allProbed;
  }

  private async probeFrames(tabId: number, allFrame: FrameInfo[]): Promise<ProbedFrame[]> {
    const allResult = await Promise.all(
      allFrame.map(async (frame) => {
        try {
          const probe = await this.browser.sendToFrame<FrameProbe>(tabId, frame.frameId, { type: "content.probe" });
          return probe ? { ...frame, probe } : undefined;
        } catch {
          return undefined;
        }
      }),
    );
    return allResult.filter((frame): frame is ProbedFrame => frame !== undefined);
  }
}
