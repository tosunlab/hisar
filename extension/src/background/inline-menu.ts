import type { InlineMenuKey, InlineMenuPurpose, InlineMenuState, OtherLogin } from "../shared/messages";
import { HisarError, toHisarError } from "../shared/protocol";
import { getHost, getOrigin, isWebUrl } from "../shared/url";
import type { BackgroundController, BrowserBridge } from "./controller";
import { InlineMenuSessionStore, type InlineMenuSession } from "./inline-menu-sessions";

// maximumOtherLoginCount bounds one search of the user's other logins; typing more narrows it.
export const maximumOtherLoginCount = 50;

export interface InlineMenuSender {
  tabId: number;
  frameId: number;
}

export class InlineMenuService {
  readonly allSession: InlineMenuSessionStore;

  constructor(
    private readonly controller: BackgroundController,
    private readonly browser: BrowserBridge,
  ) {
    this.allSession = new InlineMenuSessionStore(() => browser.now());
  }

  openSession(sender: InlineMenuSender, frameUrl: string, tabUrl: string | undefined, purpose: InlineMenuPurpose): { sessionID: string; menuUrl: string } {
    if (!isWebUrl(frameUrl) || !tabUrl || getOrigin(frameUrl) !== getOrigin(tabUrl)) {
      throw new HisarError("invalid-request", "Hisar does not fill this frame");
    }
    if (purpose === "address" && this.controller.isAddressFillRefused()) throw new HisarError("invalid-request", "Update the Hisar app to fill addresses");
    if (purpose === "identity" && this.controller.isIdentityFillRefused()) throw new HisarError("invalid-request", "Update the Hisar app to fill identities");
    if (purpose === "card" && this.controller.isCardFillRefused()) throw new HisarError("invalid-request", "Update the Hisar app to fill cards");
    const session = this.allSession.create({ sessionID: this.browser.createID(), tabId: sender.tabId, frameId: sender.frameId, frameUrl, purpose });
    return { sessionID: session.sessionID, menuUrl: `${this.browser.getInlineMenuUrl()}#${session.sessionID}` };
  }

  closeSession(sender: InlineMenuSender, sessionID: string): void {
    const session = this.allSession.get(sessionID);
    if (session && session.tabId === sender.tabId && session.frameId === sender.frameId) this.allSession.remove(sessionID);
  }

  async pressKey(sender: InlineMenuSender, sessionID: string, key: InlineMenuKey): Promise<void> {
    const session = this.allSession.get(sessionID);
    if (!session || session.tabId !== sender.tabId || session.frameId !== sender.frameId) return;
    await this.browser.broadcastToInlineMenu({ target: "inline-menu", type: "inlineMenu.pressKey", sessionID, key });
  }

  async getState(menuSender: InlineMenuSender, sessionID: string): Promise<InlineMenuState> {
    const session = this.requireSession(menuSender, sessionID);
    const base = { purpose: session.purpose, host: getHost(session.frameUrl), allLogin: [] };
    const popupState = await this.controller.getPopupState();
    if (popupState.view !== "unlocked") {
      return { ...base, view: popupState.view, errorMessage: popupState.errorMessage, updateRequired: popupState.updateRequired };
    }
    try {
      if (session.purpose === "address") {
        const allAddress = await this.controller.listAddresses();
        return { ...base, view: "logins", allAddress };
      }
      if (session.purpose === "identity") {
        const allIdentity = await this.controller.listIdentities();
        return { ...base, view: "logins", allIdentity };
      }
      if (session.purpose === "card") {
        const allCard = await this.controller.listCards();
        return { ...base, view: "logins", allCard };
      }
      if (session.purpose === "new-password") {
        session.suggestedPassword = await this.controller.generatePassword({});
        return { ...base, view: "logins", suggestedPassword: session.suggestedPassword };
      }
      let allLogin = await this.controller.matchLoginsForUrl(session.frameUrl, session.tabId);
      if (session.purpose === "one-time-code") allLogin = allLogin.filter((login) => login.hasOneTimePassword);
      // Every match goes to the menu: it shows the first few and expands in place to the rest, so no login is out of reach.
      const canFillOtherLogins = session.purpose === "login" && this.controller.isAddLoginURLAvailable();
      return { ...base, view: "logins", allLogin, ...(canFillOtherLogins ? { canFillOtherLogins } : {}) };
    } catch (error) {
      const hisarError = toHisarError(error);
      if (hisarError.code === "locked" || hisarError.code === "signed-out" || hisarError.code === "desktop-unavailable" || hisarError.code === "update-required") {
        return { ...base, view: hisarError.code };
      }
      if (hisarError.code === "not-paired") return { ...base, view: "not-paired" };
      return { ...base, view: "logins", errorMessage: hisarError.message };
    }
  }

  async fillLogin(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    await this.requireVisibleMenu(session);
    await this.fillLoginInSession(session, vaultID, itemID);
  }

  // searchOtherLogins finds the user's logins not saved for the menu's page, by query, for the menu to offer. Only the
  // menu's own page sees them (router.ts), never the page's content script.
  async searchOtherLogins(menuSender: InlineMenuSender, sessionID: string, query: string): Promise<OtherLogin[]> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "login" || !this.controller.isAddLoginURLAvailable()) return [];
    const trimmed = query.trim();
    if (trimmed === "") return [];
    const [allItem, allMatch] = await Promise.all([
      this.controller.listItems({ query: trimmed, category: "login" }),
      this.controller.matchLoginsForUrl(session.frameUrl),
    ]);
    const allMatchKey = new Set(allMatch.map((login) => `${login.vaultID}/${login.itemID}`));
    return allItem
      .filter((item) => !allMatchKey.has(`${item.vaultID}/${item.itemID}`))
      .slice(0, maximumOtherLoginCount)
      .map((item) => ({ vaultID: item.vaultID, itemID: item.itemID, title: item.title, username: item.subtitle, host: item.url ? getHost(item.url) : "" }));
  }

  // fillOtherLogin fills a login the user chose from the search and confirmed for this page: the app first adds the
  // page's site to it, so the fill goes through the same rule as every other.
  async fillOtherLogin(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "login") throw new HisarError("invalid-request", "This field does not take a login");
    await this.requireVisibleMenu(session);
    await this.controller.addLoginURL(vaultID, itemID, session.frameUrl);
    await this.fillLoginInSession(session, vaultID, itemID);
  }

  private async fillLoginInSession(session: InlineMenuSession, vaultID: string, itemID: string): Promise<void> {
    const sessionID = session.sessionID;
    const credentials = await this.controller.getCredentials(vaultID, itemID, session.frameUrl);
    const outcome = await this.browser.sendToFrame<{ filledUsername: boolean; filledPassword: boolean }>(session.tabId, session.frameId, {
      type: "content.fillLogin",
      expectedOrigin: getOrigin(session.frameUrl),
      username: credentials.username,
      password: credentials.password,
      sessionID,
    });
    if (!outcome?.filledUsername && !outcome?.filledPassword) throw new HisarError("not-found", "Hisar couldn't fill this form. Click the field and try again");
    this.allSession.remove(sessionID);
    await this.controller.recordUse(vaultID, itemID);
    await this.controller.copyOneTimeCodeAfterFill(credentials, outcome, vaultID, itemID, session.frameUrl);
  }

  async fillOneTimeCode(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "one-time-code") throw new HisarError("invalid-request", "This field does not take a one-time code");
    await this.requireVisibleMenu(session);
    const { code } = await this.controller.getOneTimeCode(vaultID, itemID, session.frameUrl);
    const isFilled = await this.browser.sendToFrame<boolean>(session.tabId, session.frameId, {
      type: "content.fillOneTimeCode",
      expectedOrigin: getOrigin(session.frameUrl),
      code,
      sessionID,
    });
    if (!isFilled) throw new HisarError("not-found", "The one-time code field is gone");
    this.allSession.remove(sessionID);
  }

  async fillAddress(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "address") throw new HisarError("invalid-request", "This field does not take an address");
    await this.requireVisibleMenu(session);
    const address = await this.controller.getAddress(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(session.tabId, session.frameId, {
      type: "content.fillAddress",
      expectedOrigin: getOrigin(session.frameUrl),
      address,
      sessionID,
    });
    if (!filledCount) throw new HisarError("not-found", "The address form is gone");
    this.allSession.remove(sessionID);
  }

  async fillIdentity(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "identity") throw new HisarError("invalid-request", "This field does not take an identity");
    await this.requireVisibleMenu(session);
    const identity = await this.controller.getIdentity(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(session.tabId, session.frameId, {
      type: "content.fillIdentity",
      expectedOrigin: getOrigin(session.frameUrl),
      identity,
      sessionID,
    });
    if (!filledCount) throw new HisarError("not-found", "The form is gone");
    this.allSession.remove(sessionID);
  }

  async fillCard(menuSender: InlineMenuSender, sessionID: string, vaultID: string, itemID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "card") throw new HisarError("invalid-request", "This field does not take a card");
    await this.requireVisibleMenu(session);
    const card = await this.controller.getCard(vaultID, itemID);
    const filledCount = await this.browser.sendToFrame<number>(session.tabId, session.frameId, {
      type: "content.fillCard",
      expectedOrigin: getOrigin(session.frameUrl),
      card,
      sessionID,
    });
    if (!filledCount) throw new HisarError("not-found", "The payment form is gone");
    this.allSession.remove(sessionID);
  }

  async useSuggestedPassword(menuSender: InlineMenuSender, sessionID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    if (session.purpose !== "new-password" || !session.suggestedPassword) throw new HisarError("invalid-request", "No suggested password");
    await this.requireVisibleMenu(session);
    const isFilled = await this.browser.sendToFrame<boolean>(session.tabId, session.frameId, {
      type: "content.fillPassword",
      expectedOrigin: getOrigin(session.frameUrl),
      password: session.suggestedPassword,
      sessionID,
    });
    if (!isFilled) throw new HisarError("not-found", "The password field is gone");
    this.allSession.remove(sessionID);
  }

  async unlock(menuSender: InlineMenuSender, sessionID: string): Promise<void> {
    this.requireSession(menuSender, sessionID);
    await this.controller.unlock();
  }

  async openHisar(menuSender: InlineMenuSender, sessionID: string): Promise<InlineMenuState> {
    this.requireSession(menuSender, sessionID);
    await this.controller.getPopupState(true);
    return this.getState(menuSender, sessionID);
  }

  async openPopup(menuSender: InlineMenuSender, sessionID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    await this.browser.sendToFrame(session.tabId, session.frameId, { type: "content.dismissInlineMenu", sessionID }).catch(() => undefined);
    await this.browser.openPopup(session.tabId);
  }

  async resize(menuSender: InlineMenuSender, sessionID: string, height: number): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    await this.browser.sendToFrame(session.tabId, session.frameId, { type: "content.resizeInlineMenu", sessionID, height });
  }

  async close(menuSender: InlineMenuSender, sessionID: string): Promise<void> {
    const session = this.requireSession(menuSender, sessionID);
    this.allSession.remove(sessionID);
    await this.browser.sendToFrame(session.tabId, session.frameId, { type: "content.dismissInlineMenu", sessionID }).catch(() => undefined);
  }

  handleTabRemoved(tabId: number): void {
    this.allSession.removeTab(tabId);
  }

  private requireSession(menuSender: InlineMenuSender, sessionID: string): InlineMenuSession {
    const session = this.allSession.claim(sessionID, menuSender.tabId, menuSender.frameId);
    if (!session) throw new HisarError("invalid-request", "This menu has expired");
    return session;
  }

  private async requireVisibleMenu(session: InlineMenuSession): Promise<void> {
    const isVisible = await this.browser
      .sendToFrame<boolean>(session.tabId, session.frameId, { type: "content.isInlineMenuVisible", sessionID: session.sessionID })
      .catch(() => false);
    if (!isVisible) throw new HisarError("invalid-request", "The menu was not fully visible. Try again");
  }
}
