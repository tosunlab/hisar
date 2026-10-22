import type {
  BackgroundToContentMessage,
  InlineMenuBroadcast,
  FillOutcome,
  FrameProbe,
  ItemQuery,
  MatchResult,
  PopupState,
  SaveOfferAction,
  SaveOfferEdits,
  SaveOfferView,
} from "../shared/messages";
import {
  featureAddresses,
  featureCards,
  featureCreateItems,
  featureIdentities,
  featureSaveConfirmation,
  featureUnlockPassword,
  featureVault,
  HisarError,
  toHisarError,
  type AddressSummary,
  type AddressValues,
  type CardSummary,
  type CardValues,
  type Credentials,
  type GenerateOptions,
  type HelloResult,
  type IdentitySummary,
  type IdentityValues,
  type ItemResult,
  type ItemSummary,
  type LoginSummary,
  type NewItem,
  type VaultSummary,
  type OneTimeCode,
  type UpdateRequirement,
} from "../shared/protocol";
import { getHost, getOrigin, isWebUrl } from "../shared/url";
import { chooseAddressFrame, chooseCardFrame, chooseFocusedFrame, chooseIdentityFrame, chooseLoginFrame, chooseOneTimeCodeFrame, selectAllFillableFrame, type FrameInfo, type ProbedFrame } from "./frames";
import type { NativeClient } from "./native-client";
import { decideSaveOffer, RememberedUsernameStore, SaveOfferStore, toSaveOfferView, type ScheduleSaveExpiry } from "./save";
import type { ExtensionStorage } from "./storage";

export interface BrowserBridge {
  getTabUrl(tabId: number): Promise<string | undefined>;
  getFrameDocumentID(tabId: number, frameId: number): Promise<string | undefined>;
  getAllFrameInfo(tabId: number): Promise<FrameInfo[]>;
  injectContentScript(tabId: number): Promise<void>;
  sendToFrame<Result>(tabId: number, frameId: number, message: BackgroundToContentMessage): Promise<Result>;
  broadcastToInlineMenu(message: InlineMenuBroadcast): Promise<void>;
  openPopup(tabId: number): Promise<void>;
  setSaveBadge?(tabId: number, isPending: boolean): Promise<void>;
  scheduleSaveExpiry?: ScheduleSaveExpiry;
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
  private allAppFeature: string[] = [];
  private readonly neverOrigins = new Set<string>();
  // hasHello: this service worker has said hello to the app, so allAppFeature is the app's. The browser stops an idle
  // worker, and pages keep the features of an earlier hello, so a page may ask for an address menu before the first one.
  private hasHello = false;

  constructor(
    private readonly client: NativeClient,
    private readonly storage: ExtensionStorage,
    private readonly browser: BrowserBridge,
  ) {
    this.allSaveOffer = new SaveOfferStore(
      () => browser.now(),
      120000,
      tabId => { void browser.setSaveBadge?.(tabId, false); },
      browser.scheduleSaveExpiry,
    );
    this.allRememberedUsername = new RememberedUsernameStore(() => browser.now());
  }

  async handleHello(result: HelloResult): Promise<void> {
    this.updateRequirement = result.updateRequired;
    this.allAppFeature = result.features ?? [];
    this.hasHello = true;
    if (!result.isPaired || result.state !== "unlocked") this.allSaveOffer.clear();
    // Pages read the features to offer address filling only with an app that does it.
    await this.storage.setAppFeatures(this.allAppFeature);
    if (!result.isPaired) await this.storage.removeToken();
  }

  isAddressFillAvailable(): boolean {
    return this.allAppFeature.includes(featureAddresses);
  }

  // isAddressFillRefused reports whether the app said it does not fill addresses. Before this worker's first hello it does
  // not know, so the inline menu opens and its first request connects; requireAddressFill then decides.
  isAddressFillRefused(): boolean {
    return this.hasHello && !this.isAddressFillAvailable();
  }

  // isOverOpenConnection asks only over a connection that is already open, as the popup does while it shows the vault: a
  // connection would start the host, which would relaunch Hisar after the user quit it.
  async getPopupState(shouldReconnect = false, isOverOpenConnection = false): Promise<PopupState> {
    let hello: HelloResult;
    try {
      if (isOverOpenConnection) {
        hello = await this.client.waitForOpenConnection();
      } else {
        const hasToken = (await this.storage.getToken()) !== undefined;
        hello = shouldReconnect || !hasToken ? await this.client.reconnect() : await this.client.connect();
      }
    } catch (error) {
      return { view: "desktop-unavailable", errorMessage: toHisarError(error).message };
    }
    if (hello.updateRequired) return { view: "update-required", updateRequired: hello.updateRequired };
    if (!(await this.storage.getToken())) return { view: hello.state === "signed-out" ? "signed-out" : "not-paired" };
    try {
      const status = await this.client.request("status", {}, { isOverOpenConnection });
      if (status.state !== "unlocked") this.allSaveOffer.clear();
      return {
        view: status.state,
        email: status.email,
        isOffline: status.isOffline,
        isAddressFillAvailable: this.isAddressFillAvailable(),
        isCardFillAvailable: this.isCardFillAvailable(),
        isIdentityFillAvailable: this.isIdentityFillAvailable(),
        isVaultAvailable: this.isVaultAvailable(),
        isCreateAvailable: this.allAppFeature.includes(featureCreateItems),
        isPasswordUnlockAvailable: this.isPasswordUnlockAvailable(),
        isSaveConfirmationAvailable: this.allAppFeature.includes(featureSaveConfirmation),
      };
    } catch (error) {
      const hisarError = toHisarError(error);
      if (hisarError.code === "not-paired") {
        await this.storage.removeToken();
        return { view: "not-paired" };
      }
      if (hisarError.code === "signed-out") return { view: hisarError.code };
      if (hisarError.code === "locked") return { view: hisarError.code, isPasswordUnlockAvailable: this.isPasswordUnlockAvailable() };
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

  isPasswordUnlockAvailable(): boolean {
    return this.allAppFeature.includes(featureUnlockPassword);
  }

  // unlockWithPassword passes the master password the user typed in the popup to the app, which derives the keys from
  // it; the extension keeps no copy.
  async unlockWithPassword(password: string): Promise<void> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.isPasswordUnlockAvailable()) throw new HisarError("invalid-request", "Update the Hisar app to unlock from the browser");
    if (!password) throw new HisarError("invalid-request", "Enter your master password");
    await this.request(() => this.client.request("unlock.password", { password }));
  }

  async getTabUrl(tabId: number): Promise<string | undefined> {
    return this.browser.getTabUrl(tabId);
  }

  // matchLoginsForUrl returns the logins for url with those filled most recently first; the rest keep Hisar's order. The
  // popup, the inline menu and the fill shortcut all read this order, so the first login is the same in each.
  async matchLoginsForUrl(url: string): Promise<LoginSummary[]> {
    if (!isWebUrl(url)) return [];
    const result = await this.request(() => this.client.request("logins.match", { url }));
    return this.sortByLastUse(result.logins ?? []);
  }

  private async sortByLastUse(allLogin: LoginSummary[]): Promise<LoginSummary[]> {
    const allLastUsed = await this.storage.getAllLastUsed().catch(() => ({}) as Record<string, number>);
    const lastUsed = (login: LoginSummary): number => allLastUsed[`${login.vaultID}/${login.itemID}`] ?? 0;
    return [...allLogin].sort((first, second) => lastUsed(second) - lastUsed(first));
  }

  async getCredentials(vaultID: string, itemID: string, url: string): Promise<Credentials> {
    return this.request(() => this.client.request("logins.fill", { vaultID, itemID, url }));
  }

  async matchLogins(tabId: number): Promise<MatchResult> {
    const url = await this.browser.getTabUrl(tabId);
    if (!isWebUrl(url)) return { url: url ?? "", allLogin: [] };
    return { url, allLogin: await this.matchLoginsForUrl(url) };
  }

  async getOneTimeCode(vaultID: string, itemID: string, url: string): Promise<OneTimeCode> {
    return this.request(() => this.client.request("logins.code", { vaultID, itemID, url }));
  }

  async getOneTimeCodeForTab(tabId: number, vaultID: string, itemID: string): Promise<OneTimeCode> {
    const url = await this.browser.getTabUrl(tabId);
    if (!isWebUrl(url)) throw new HisarError("no-match", "This page has no web address");
    return this.getOneTimeCode(vaultID, itemID, url);
  }

  // copyOneTimeCodeAfterFill puts the code on the clipboard for the next step of the sign-in once the password is filled;
  // after only a username the code is not needed yet.
  async copyOneTimeCodeAfterFill(credentials: Credentials, outcome: { filledPassword: boolean }, vaultID: string, itemID: string, url: string): Promise<boolean> {
    if (!credentials.hasOneTimePassword || !outcome.filledPassword) return false;
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
    await this.recordUse(vaultID, itemID);
    await this.copyOneTimeCodeAfterFill(credentials, outcome, vaultID, itemID, target.url);
    return outcome;
  }

  async recordUse(vaultID: string, itemID: string): Promise<void> {
    await this.storage.recordUse(`${vaultID}/${itemID}`, this.browser.now()).catch(() => undefined);
  }

  getAllLastUsed(): Promise<Record<string, number>> {
    return this.storage.getAllLastUsed();
  }

  // getBadgeText is the number of logins saved for the tab, for the toolbar icon. It asks only over a connection that is
  // already open, so switching tabs never starts the native host, which would start Hisar.
  async getBadgeText(tabId: number): Promise<string> {
    if (!this.client.isConnected() || !(await this.storage.getToken())) return "";
    if (this.allSaveOffer.getForTab(tabId, await this.storage.getToken())) return "+";
    try {
      const url = await this.browser.getTabUrl(tabId);
      if (!isWebUrl(url)) return "";
      const allLogin = await this.matchLoginsForUrl(url);
      if (allLogin.length === 0) return "";
      return allLogin.length > 9 ? "9+" : String(allLogin.length);
    } catch {
      return "";
    }
  }

  // fillSuggestedPassword generates a password and fills it into the focused field, for the right-click menu.
  async fillSuggestedPassword(tabId: number): Promise<void> {
    await this.fillPassword(tabId, await this.generatePassword({}));
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

  async listAddresses(): Promise<AddressSummary[]> {
    await this.requireAddressFill();
    const result = await this.request(() => this.client.request("addresses.list", {}));
    return result.addresses ?? [];
  }

  async getAddress(vaultID: string, itemID: string): Promise<AddressValues> {
    await this.requireAddressFill();
    return this.request(() => this.client.request("addresses.fill", { vaultID, itemID }));
  }

  // fillAddress fills the address form with the focused field, or else the page's first one. It looks for the form before
  // it asks Hisar for the address, so a page without one never receives it.
  async fillAddress(tabId: number, vaultID: string, itemID: string): Promise<number> {
    await this.requireAddressFill();
    const target = chooseAddressFrame(await this.probeFillableFrames(tabId));
    if (!target) throw new HisarError("not-found", "No address form found on this page");
    const address = await this.getAddress(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(tabId, target.frameId, { type: "content.fillAddress", expectedOrigin: target.origin, address });
    if (!filledCount) throw new HisarError("not-found", "No empty address field found on this page");
    return filledCount;
  }

  private async requireAddressFill(): Promise<void> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.isAddressFillAvailable()) throw new HisarError("invalid-request", "Update the Hisar app to fill addresses");
  }

  isVaultAvailable(): boolean {
    return this.allAppFeature.includes(featureVault);
  }

  // The vault requests are answered only to the extension's own pages (router.ts): a page's content script never sees
  // titles of other sites, or a secret it did not ask to fill.
  async listItems(query: ItemQuery): Promise<ItemSummary[]> {
    await this.requireVault();
    const result = await this.request(() => this.client.request("items.list", query));
    return result.items ?? [];
  }

  async getItem(vaultID: string, itemID: string): Promise<ItemResult> {
    await this.requireVault();
    return this.request(() => this.client.request("items.get", { vaultID, itemID }));
  }

  async revealItemField(vaultID: string, itemID: string, fieldID: string): Promise<string> {
    await this.requireVault();
    const result = await this.request(() => this.client.request("items.reveal", { vaultID, itemID, fieldID }));
    return result.value;
  }

  async getItemCode(vaultID: string, itemID: string): Promise<OneTimeCode> {
    await this.requireVault();
    return this.request(() => this.client.request("items.code", { vaultID, itemID }));
  }

  // getItemIcon returns the website's icon as a data: URL, or "" when there is none.
  async getItemIcon(url: string): Promise<string> {
    if (!isWebUrl(url)) return "";
    await this.requireVault();
    const result = await this.request(() => this.client.request("items.icon", { url }));
    return result.png ? `data:image/png;base64,${result.png}` : "";
  }

  async setItemFavorite(vaultID: string, itemID: string, isFavorite: boolean): Promise<void> {
    await this.requireVault();
    await this.request(() => this.client.request("items.favorite", { vaultID, itemID, isFavorite }));
  }

  private async requireVault(): Promise<void> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.isVaultAvailable()) throw new HisarError("invalid-request", "Update the Hisar app to browse your vaults");
  }

  isIdentityFillAvailable(): boolean {
    return this.allAppFeature.includes(featureIdentities);
  }

  isIdentityFillRefused(): boolean {
    return this.hasHello && !this.isIdentityFillAvailable();
  }

  async listIdentities(): Promise<IdentitySummary[]> {
    await this.requireIdentityFill();
    const result = await this.request(() => this.client.request("identities.list", {}));
    return result.identities ?? [];
  }

  async getIdentity(vaultID: string, itemID: string): Promise<IdentityValues> {
    await this.requireIdentityFill();
    return this.request(() => this.client.request("identities.fill", { vaultID, itemID }));
  }

  // fillIdentity fills the form of personal details with the focused field, or else the page's first one, and the address
  // form beside it. It looks for the form before it asks Hisar for the identity.
  async fillIdentity(tabId: number, vaultID: string, itemID: string): Promise<number> {
    await this.requireIdentityFill();
    const target = chooseIdentityFrame(await this.probeFillableFrames(tabId));
    if (!target) throw new HisarError("not-found", "No name, email or phone field found on this page");
    const identity = await this.getIdentity(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(tabId, target.frameId, { type: "content.fillIdentity", expectedOrigin: target.origin, identity });
    if (!filledCount) throw new HisarError("not-found", "No empty field for this identity found on this page");
    return filledCount;
  }

  private async requireIdentityFill(): Promise<void> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.isIdentityFillAvailable()) throw new HisarError("invalid-request", "Update the Hisar app to fill identities");
  }

  isCardFillAvailable(): boolean {
    return this.allAppFeature.includes(featureCards);
  }

  // isCardFillRefused, like isAddressFillRefused, is known only after this worker's first hello.
  isCardFillRefused(): boolean {
    return this.hasHello && !this.isCardFillAvailable();
  }

  async listCards(): Promise<CardSummary[]> {
    await this.requireCardFill();
    const result = await this.request(() => this.client.request("cards.list", {}));
    return result.cards ?? [];
  }

  async getCard(vaultID: string, itemID: string): Promise<CardValues> {
    await this.requireCardFill();
    return this.request(() => this.client.request("cards.fill", { vaultID, itemID }));
  }

  // fillCard fills the payment form with the focused field, or else the page's first one, in the tab's own frames only.
  // It looks for the form before it asks Hisar for the card, so a page without one never receives it.
  async fillCard(tabId: number, vaultID: string, itemID: string): Promise<number> {
    await this.requireCardFill();
    const target = chooseCardFrame(await this.probeFillableFrames(tabId));
    if (!target) throw new HisarError("not-found", "No payment form found on this page");
    const card = await this.getCard(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(tabId, target.frameId, { type: "content.fillCard", expectedOrigin: target.origin, card });
    if (!filledCount) throw new HisarError("not-found", "No empty card field found on this page");
    return filledCount;
  }

  private async requireCardFill(): Promise<void> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.isCardFillAvailable()) throw new HisarError("invalid-request", "Update the Hisar app to fill cards");
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

  async handleUsernameSubmitted(tabId: number, frameUrl: string, username: string): Promise<void> {
    if (!isWebUrl(frameUrl) || !username) return;
    const pairingToken = await this.storage.getToken();
    if (pairingToken) this.allRememberedUsername.remember(tabId, getOrigin(frameUrl), username, pairingToken);
  }

  async handleLoginSubmitted(tabId: number, frameUrl: string, username: string, password: string, frameId = 0, tabURL = frameUrl, documentID?: string): Promise<SaveOfferView | undefined> {
    if (!isWebUrl(frameUrl) || !password) return undefined;
    const navigationID = this.browser.createID();
    const capturedAt = this.browser.now();
    this.allSaveOffer.beginSubmission(tabId, navigationID, tabURL, frameId, frameUrl, documentID, capturedAt);
    void this.browser.setSaveBadge?.(tabId, false);
    if (documentID && !this.allSaveOffer.matchesDocument(tabId, navigationID, await this.browser.getFrameDocumentID(tabId, frameId))) return undefined;
    const origin = getOrigin(frameUrl);
    if (await this.storage.isNeverOrigin(origin)) return undefined;
    const pairingToken = await this.storage.getToken();
    if (!pairingToken) return undefined;
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.allAppFeature.includes(featureSaveConfirmation)) return undefined;
    const effectiveUsername = username || this.allRememberedUsername.recall(tabId, origin, pairingToken) || "";
    let checkResult;
    try {
      checkResult = await this.request(
        () => this.client.request("logins.prepare", { url: frameUrl, username: effectiveUsername, password }, { pairingToken }),
        pairingToken,
      );
    } catch {
      return undefined;
    }
    const kind = decideSaveOffer(checkResult, false);
    if (!kind || !checkResult.saveContext) return undefined;
    const allVault = await this.request(() => this.client.request("vaults.list", {}, { pairingToken }), pairingToken)
      .then((result) => (result.vaults ?? []).sort((a, b) => Number(b.isPersonal) - Number(a.isPersonal)))
      .catch(() => undefined);
    const offer = {
      offerID: this.browser.createID(),
      pairingToken,
      saveContext: checkResult.saveContext,
      navigationID,
      vaultName: checkResult.vaultName ?? allVault?.find((vault) => vault.vaultID === checkResult.vaultID)?.name,
      isPersonal: checkResult.isPersonal ?? allVault?.find((vault) => vault.vaultID === checkResult.vaultID)?.isPersonal,
      tabId,
      url: frameUrl,
      host: getHost(frameUrl),
      username: effectiveUsername,
      password,
      kind,
      vaultID: kind === "update" ? checkResult.vaultID : undefined,
      itemID: kind === "update" ? checkResult.itemID : undefined,
      title: checkResult.title,
      defaultTitle: allVault ? getHost(frameUrl).replace(/^www\./, "") : undefined,
      allVault,
      createdAt: capturedAt,
    };
    if (!this.allSaveOffer.isCurrent(tabId, navigationID) || this.neverOrigins.has(origin) || await this.storage.isNeverOrigin(origin) || (await this.storage.getToken()) !== pairingToken) return undefined;
    if (!this.allSaveOffer.put(offer)) return undefined;
    await this.browser.setSaveBadge?.(tabId, true);
    return toSaveOfferView(offer);
  }

  async getPendingSaveOffer(tabId: number): Promise<SaveOfferView | null> {
    const offer = this.allSaveOffer.getForTab(tabId, await this.storage.getToken());
    if (!offer) {
      await this.browser.setSaveBadge?.(tabId, false);
      return null;
    }
    try {
      await this.request(() => this.client.request("logins.review", { saveContext: offer.saveContext }, { pairingToken: offer.pairingToken }), offer.pairingToken);
    } catch {
      this.allSaveOffer.take(tabId, offer.offerID);
      await this.browser.setSaveBadge?.(tabId, false);
      return null;
    }
    return this.allSaveOffer.getForTab(tabId, offer.pairingToken) === offer ? toSaveOfferView(offer) : null;
  }

  async actOnSaveOffer(tabId: number, offerID: string, action: SaveOfferAction, edits?: SaveOfferEdits): Promise<void> {
    const offer = this.allSaveOffer.getForTab(tabId, await this.storage.getToken());
    if (!offer || offer.offerID !== offerID) throw new HisarError("not-found", "This save offer has expired");
    if (action === "not-now") {
      this.allSaveOffer.take(tabId, offerID);
      await this.browser.setSaveBadge?.(tabId, false);
      return;
    }
    if (action === "never") {
      this.neverOrigins.add(getOrigin(offer.url));
      this.allSaveOffer.take(tabId, offerID);
      await this.storage.addNeverOrigin(getOrigin(offer.url));
      await this.browser.setSaveBadge?.(tabId, false);
      return;
    }
    const parameters =
      offer.kind === "update" && offer.vaultID && offer.itemID
        ? { url: offer.url, username: offer.username, password: offer.password, vaultID: offer.vaultID, itemID: offer.itemID }
        : { url: offer.url, username: offer.username, password: offer.password, ...this.applySaveEdits(offer.allVault, edits) };
    const tabURL = await this.browser.getTabUrl(tabId);
    if (!tabURL || !offer.navigationID || this.neverOrigins.has(getOrigin(offer.url)) || !this.allSaveOffer.isCurrent(tabId, offer.navigationID, tabURL)) throw new HisarError("not-found", "This page has changed; submit the login again");
    await this.request(() => this.client.request("logins.save", { ...parameters, saveContext: offer.saveContext }, {
      pairingToken: offer.pairingToken,
      isStillValid: () => !this.neverOrigins.has(getOrigin(offer.url)) && this.allSaveOffer.getForTab(tabId, offer.pairingToken) === offer && this.allSaveOffer.take(tabId, offerID) === offer,
    }), offer.pairingToken);
    await this.browser.setSaveBadge?.(tabId, false);
  }

  private applySaveEdits(allVault: { vaultID: string }[] | undefined, edits: SaveOfferEdits | undefined): { username?: string; title?: string; vaultID?: string } {
    if (!edits) return {};
    const changes: { username?: string; title?: string; vaultID?: string } = { username: edits.username.trim() };
    if (edits.title.trim()) changes.title = edits.title.trim();
    if (edits.vaultID && allVault?.some((vault) => vault.vaultID === edits.vaultID)) changes.vaultID = edits.vaultID;
    return changes;
  }

  async listVaults(): Promise<VaultSummary[]> {
    await this.requireVault();
    const result = await this.request(() => this.client.request("vaults.list", {}));
    return result.vaults ?? [];
  }

  async createItem(vaultID: string, item: NewItem): Promise<ItemSummary> {
    if (!this.hasHello) await this.request(() => this.client.connect());
    if (!this.allAppFeature.includes(featureCreateItems)) throw new HisarError("invalid-request", "Update the Hisar app to add items from the browser");
    return this.request(() => this.client.request("items.create", { vaultID, item }));
  }

  handleTabNavigation(tabId: number, url: string, frameId = 0, documentID?: string): void {
    this.allSaveOffer.navigate(tabId, url, frameId, documentID);
    void this.browser.setSaveBadge?.(tabId, false);
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

  // request sends a request and forgets the pairing Hisar no longer knows; with pairingToken, the request was sent only under
  // that pairing, and the browser's pairing is forgotten only if it is still that one.
  private async request<Result>(send: () => Promise<Result>, pairingToken?: string): Promise<Result> {
    if (this.updateRequirement?.target === "extension") {
      throw new HisarError("update-required", `Update the Hisar extension: this server requires version ${this.updateRequirement.minimumVersion} or later.`);
    }
    try {
      return await send();
    } catch (error) {
      const hisarError = toHisarError(error);
      if (["not-paired", "locked", "signed-out"].includes(hisarError.code) && (pairingToken === undefined || (await this.storage.getToken()) === pairingToken)) {
        this.allSaveOffer.clear();
        if (hisarError.code === "not-paired") await this.storage.removeToken();
      }
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
