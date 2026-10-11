import type { InlineMenuKey, InlineMenuPurpose, InlineMenuState } from "../shared/messages";
import { HisarError, toHisarError } from "../shared/protocol";
import { getHost, getOrigin, isWebUrl } from "../shared/url";
import type { BackgroundController, BrowserBridge } from "./controller";
import { InlineMenuSessionStore, type InlineMenuSession } from "./inline-menu-sessions";

export const maximumInlineLoginCount = 6;

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
    const base = { purpose: session.purpose, host: getHost(session.frameUrl), allLogin: [], hasMoreLogin: false };
    const popupState = await this.controller.getPopupState();
    if (popupState.view !== "unlocked") {
      return { ...base, view: popupState.view, errorMessage: popupState.errorMessage, updateRequired: popupState.updateRequired };
    }
    try {
      if (session.purpose === "new-password") {
        session.suggestedPassword = await this.controller.generatePassword({});
        return { ...base, view: "logins", suggestedPassword: session.suggestedPassword };
      }
      let allLogin = await this.controller.matchLoginsForUrl(session.frameUrl);
      if (session.purpose === "one-time-code") allLogin = allLogin.filter((login) => login.hasOneTimePassword);
      return { ...base, view: "logins", allLogin: allLogin.slice(0, maximumInlineLoginCount), hasMoreLogin: allLogin.length > maximumInlineLoginCount };
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
    const credentials = await this.controller.getCredentials(vaultID, itemID, session.frameUrl);
    const outcome = await this.browser.sendToFrame<{ filledUsername: boolean; filledPassword: boolean }>(session.tabId, session.frameId, {
      type: "content.fillLogin",
      expectedOrigin: getOrigin(session.frameUrl),
      username: credentials.username,
      password: credentials.password,
      sessionID,
    });
    if (!outcome?.filledUsername && !outcome?.filledPassword) throw new HisarError("not-found", "The login field is gone");
    this.allSession.remove(sessionID);
    await this.controller.copyOneTimeCodeAfterFill(credentials, vaultID, itemID, session.frameUrl);
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
