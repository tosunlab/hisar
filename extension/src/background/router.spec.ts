import { describe, expect, it, vi } from "vitest";
import { HisarError } from "../shared/protocol";
import type { BackgroundController } from "./controller";
import { routeMessage } from "./router";

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

describe("routeMessage", () => {
  it("routes popup messages from extension pages", async () => {
    const controller = createController();
    expect(await routeMessage(controller, { type: "popup.getState" }, popupSender, context)).toEqual({ ok: true, result: { view: "unlocked" } });
    await routeMessage(controller, { type: "popup.fillLogin", tabId: 3, vaultID: "v", itemID: "i" } as never, popupSender, context);
    expect(controller.fillLogin).toHaveBeenCalledWith(3, "v", "i");
  });

  it("refuses popup messages from content scripts", async () => {
    const controller = createController();
    const response = await routeMessage(controller, { type: "popup.unlock" }, contentSender, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
    expect(controller.unlock).not.toHaveBeenCalled();
  });

  it("refuses messages from other extensions", async () => {
    const response = await routeMessage(createController(), { type: "popup.getState" }, { ...popupSender, id: "other" }, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
  });

  it("takes the tab and URL of content messages from the sender, not the message", async () => {
    const controller = createController();
    await routeMessage(
      controller,
      { type: "content.loginSubmitted", username: "alex", password: "pw", url: "https://attacker.example" } as never,
      contentSender,
      context,
    );
    expect(controller.handleLoginSubmitted).toHaveBeenCalledWith(9, "https://example.com/login", "alex", "pw");
  });

  it("refuses content messages without a tab", async () => {
    const response = await routeMessage(createController(), { type: "content.getPendingSaveOffer" }, popupSender, context);
    expect(response).toMatchObject({ ok: false, error: { code: "invalid-request" } });
  });

  it("returns errors with their codes", async () => {
    const response = await routeMessage(
      createController(),
      { type: "content.actOnSaveOffer", offerID: "x", action: "save" } as never,
      contentSender,
      context,
    );
    expect(response).toEqual({ ok: false, error: { code: "not-found", message: "This save offer has expired" } });
  });

  it("leaves offscreen messages and unknown shapes alone", async () => {
    expect(await routeMessage(createController(), { type: "offscreen.copy", target: "offscreen" }, popupSender, context)).toBeUndefined();
    expect(await routeMessage(createController(), {}, popupSender, context)).toBeUndefined();
    expect(await routeMessage(createController(), { type: "other.thing" }, popupSender, context)).toMatchObject({ ok: false });
  });
});
