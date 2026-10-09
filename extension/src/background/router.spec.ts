import { describe, expect, it, vi } from "vitest";
import { HisarError } from "../shared/protocol";
import type { BackgroundController } from "./controller";
import type { InlineMenuService } from "./inline-menu";
import { routeMessage, type RouterServices } from "./router";

const context = { extensionID: "mmipiffoogblblocokimpemdopmdjghf", extensionOrigin: "chrome-extension://mmipiffoogblblocokimpemdopmdjghf/" };
const popupSender = { id: context.extensionID, url: `${context.extensionOrigin}popup.html` };
const contentSender = { id: context.extensionID, url: "https://example.com/login", tab: { id: 9 }, frameId: 0 };

function createController(): BackgroundController {
  return {
    getPopupState: vi.fn(async () => ({ view: "unlocked" })),
    fillLogin: vi.fn(async () => ({ filledUsername: true, filledPassword: true })),
    handleLoginSubmitted: vi.fn(async () => undefined),
    handleUsernameSubmitted: vi.fn(),
    getPendingSaveOffer: vi.fn(() => null),
    actOnSaveOffer: vi.fn(async () => {
      throw new HisarError("not-found", "This save offer has expired");
    }),
    unlock: vi.fn(async () => undefined),
  } as unknown as BackgroundController;
}

function createServices(controller: BackgroundController): RouterServices {
  return { controller, inlineMenu: {} as InlineMenuService };
}

describe("routeMessage", () => {
  it("routes popup messages from extension pages", async () => {
    const controller = createController();
    expect(await routeMessage(createServices(controller), { type: "popup.getState" }, popupSender, context)).toEqual({ ok: true, result: { view: "unlocked" } });
    await routeMessage(createServices(controller), { type: "popup.fillLogin", tabId: 3, vaultID: "v", itemID: "i" } as never, popupSender, context);
    expect(controller.fillLogin).toHaveBeenCalledWith(3, "v", "i");
  });

  it("refuses popup messages from content scripts", async () => {
    const controller = createController();
    const response = await routeMessage(createServices(controller), { type: "popup.unlock" }, contentSender, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
    expect(controller.unlock).not.toHaveBeenCalled();
  });

  it("refuses messages from other extensions", async () => {
    const response = await routeMessage(createServices(createController()), { type: "popup.getState" }, { ...popupSender, id: "other" }, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
  });

  it("takes the tab and URL of content messages from the sender, not the message", async () => {
    const controller = createController();
    await routeMessage(
      createServices(controller),
      { type: "content.loginSubmitted", username: "alex", password: "pw", url: "https://attacker.example" } as never,
      contentSender,
      context,
    );
    expect(controller.handleLoginSubmitted).toHaveBeenCalledWith(9, "https://example.com/login", "alex", "pw");
  });

  it("refuses content messages without a tab", async () => {
    const response = await routeMessage(createServices(createController()), { type: "content.getPendingSaveOffer" }, popupSender, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
  });

  it("returns errors with their codes", async () => {
    const response = await routeMessage(
      createServices(createController()),
      { type: "content.actOnSaveOffer", offerID: "x", action: "save" } as never,
      contentSender,
      context,
    );
    expect(response).toEqual({ ok: false, error: { code: "not-found", message: "This save offer has expired" } });
  });

  it("leaves offscreen messages and unknown shapes alone", async () => {
    expect(await routeMessage(createServices(createController()), { type: "offscreen.copy", target: "offscreen" }, popupSender, context)).toBeUndefined();
    expect(await routeMessage(createServices(createController()), {}, popupSender, context)).toBeUndefined();
    expect(await routeMessage(createServices(createController()), { type: "other.thing" }, popupSender, context)).toMatchObject({ ok: false });
  });

  it("routes inline menu messages only from the inline menu page inside a tab", async () => {
    const getState = vi.fn(async () => ({ view: "logins" }));
    const services = { controller: createController(), inlineMenu: { getState } as unknown as InlineMenuService };
    const menuSender = { id: context.extensionID, url: "chrome-extension://da158d19-dynamic/inline-menu.html#s", tab: { id: 9 }, frameId: 4 };
    expect(await routeMessage(services, { type: "inlineMenu.getState", sessionID: "s" } as never, menuSender, context)).toEqual({
      ok: true,
      result: { view: "logins" },
    });
    expect(getState).toHaveBeenCalledWith({ tabId: 9, frameId: 4 }, "s");
    for (const sender of [contentSender, { ...menuSender, url: `${context.extensionOrigin}popup.html` }, { ...menuSender, tab: undefined }]) {
      expect(await routeMessage(services, { type: "inlineMenu.getState", sessionID: "s" } as never, sender, context)).toMatchObject({
        ok: false,
        error: { code: "invalid-request" },
      });
    }
    expect(getState).toHaveBeenCalledTimes(1);
  });

  it("opens inline menu sessions with the frame and tab URLs from the sender", async () => {
    const openSession = vi.fn(() => ({ sessionID: "s", menuUrl: "chrome-extension://x/inline-menu.html#s" }));
    const services = { controller: createController(), inlineMenu: { openSession } as unknown as InlineMenuService };
    await routeMessage(services, { type: "content.openInlineMenu", purpose: "login" } as never, { ...contentSender, tab: { id: 9, url: "https://example.com/" }, frameId: 3 }, context);
    expect(openSession).toHaveBeenCalledWith({ tabId: 9, frameId: 3 }, "https://example.com/login", "https://example.com/", "login");
  });
});
