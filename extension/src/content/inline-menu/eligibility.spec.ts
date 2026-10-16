import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import addressAutocompletePage from "../fixtures/address-autocomplete.html?raw";
import changePasswordPage from "../fixtures/change-password.html?raw";
import checkoutLoginMixedPage from "../fixtures/checkout-login-mixed.html?raw";
import emailFirstStepOnePage from "../fixtures/email-first-step-one.html?raw";
import signUpPage from "../fixtures/sign-up.html?raw";
import simpleLoginPage from "../fixtures/simple-login.html?raw";
import twoFactorPage from "../fixtures/two-factor.html?raw";
import { findInlineMenuPurpose, isFrameEligibleForInlineMenu } from "./eligibility";
import type { Rect } from "./placement";

const wideRect = (): Rect => ({ left: 0, top: 0, width: 300, height: 36 });

function getInput(selector: string): HTMLInputElement {
  return document.querySelector<HTMLInputElement>(selector)!;
}

function createFakeWindow(href: string, allAncestorOrigin: string[] = []): Window {
  const url = new URL(href);
  return { location: { protocol: url.protocol, origin: url.origin, ancestorOrigins: allAncestorOrigin } } as unknown as Window;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("isFrameEligibleForInlineMenu", () => {
  it("allows the top frame and frames whose ancestors all share its origin", () => {
    expect(isFrameEligibleForInlineMenu(createFakeWindow("https://example.com/login"))).toBe(true);
    expect(isFrameEligibleForInlineMenu(createFakeWindow("https://example.com/frame", ["https://example.com"]))).toBe(true);
  });

  it("refuses cross-origin frames, nested cross-origin chains and non-web pages", () => {
    expect(isFrameEligibleForInlineMenu(createFakeWindow("https://login.example.net/", ["https://example.com"]))).toBe(false);
    expect(isFrameEligibleForInlineMenu(createFakeWindow("https://example.com/", ["https://ads.example.net", "https://example.com"]))).toBe(false);
    expect(isFrameEligibleForInlineMenu(createFakeWindow("file:///tmp/login.html"))).toBe(false);
  });
});

describe("findInlineMenuPurpose", () => {
  it("offers logins on the username and password fields of a login form", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(findInlineMenuPurpose(document, getInput("#login_field"), wideRect)).toBe("login");
    expect(findInlineMenuPurpose(document, getInput("#password"), wideRect)).toBe("login");
  });

  it("offers logins on the first step of an email-first sign-in", () => {
    document.body.innerHTML = emailFirstStepOnePage;
    expect(findInlineMenuPurpose(document, getInput("#identifierId"), wideRect)).toBe("login");
  });

  it("ignores search and newsletter fields", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(findInlineMenuPurpose(document, getInput("[name=q]"), wideRect)).toBeUndefined();
    expect(findInlineMenuPurpose(document, getInput("[name=newsletter_email]"), wideRect)).toBeUndefined();
  });

  it("suggests passwords on the new-password fields of sign-up and change-password forms", () => {
    document.body.innerHTML = signUpPage;
    expect(findInlineMenuPurpose(document, getInput("#new-password"), wideRect)).toBe("new-password");
    expect(findInlineMenuPurpose(document, getInput("#confirm-password"), wideRect)).toBe("new-password");
    expect(findInlineMenuPurpose(document, getInput("[name=email]"), wideRect)).toBeUndefined();
    document.body.innerHTML = changePasswordPage;
    expect(findInlineMenuPurpose(document, getInput("#current"), wideRect)).toBeUndefined();
    expect(findInlineMenuPurpose(document, getInput("#next-password"), wideRect)).toBe("new-password");
  });

  it("skips disabled, read-only and hidden fields", () => {
    document.body.innerHTML = `<form><input id="user" autocomplete="username" /><input id="pass" type="password" /></form>`;
    getInput("#user").readOnly = true;
    expect(findInlineMenuPurpose(document, getInput("#user"), wideRect)).toBeUndefined();
    getInput("#pass").disabled = true;
    expect(findInlineMenuPurpose(document, getInput("#pass"), wideRect)).toBeUndefined();
    document.body.innerHTML = `<form><input id="user" autocomplete="username" style="display:none" /><input id="pass" type="password" /></form>`;
    expect(findInlineMenuPurpose(document, getInput("#user"), wideRect)).toBeUndefined();
  });

  it("skips tiny fields when the page has layout", () => {
    document.body.innerHTML = simpleLoginPage;
    vi.spyOn(document.documentElement, "getClientRects").mockReturnValue([{}] as unknown as DOMRectList);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 300, height: 36 } as DOMRect);
    expect(findInlineMenuPurpose(document, getInput("#password"), () => ({ left: 0, top: 0, width: 40, height: 36 }))).toBeUndefined();
    expect(findInlineMenuPurpose(document, getInput("#password"), wideRect)).toBe("login");
  });

  it("never offers itself on inputs inside Hisar's own elements", () => {
    const host = document.createElement("hisar-save-bar");
    document.body.append(host);
    const shadowRoot = host.attachShadow({ mode: "open" });
    shadowRoot.innerHTML = `<form><input id="user" autocomplete="username" /><input id="pass" type="password" /></form>`;
    expect(findInlineMenuPurpose(document, shadowRoot.getElementById("pass")!, wideRect)).toBeUndefined();
  });

  it("offers one-time codes on a code field and on split code boxes as a whole", () => {
    document.body.innerHTML = twoFactorPage;
    expect(findInlineMenuPurpose(document, getInput("#app_totp"), wideRect)).toBe("one-time-code");
    document.body.innerHTML = `<form><input id="user" autocomplete="username" /><input id="pass" type="password" /><input id="otp" name="otp" /></form>`;
    expect(findInlineMenuPurpose(document, getInput("#otp"), wideRect)).toBe("one-time-code");
    expect(findInlineMenuPurpose(document, getInput("#pass"), wideRect)).toBe("login");
    document.body.innerHTML = `<div>${Array.from({ length: 6 }, (_, index) => `<input id="box-${index}" maxlength="1" />`).join("")}</div>`;
    vi.spyOn(document.documentElement, "getClientRects").mockReturnValue([{}] as unknown as DOMRectList);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, width: 40, height: 40 } as DOMRect);
    const getBoxRect = (element: Element): Rect => ({ left: Number(element.id.slice(4)) * 48, top: 0, width: 40, height: 40 });
    expect(findInlineMenuPurpose(document, getInput("#box-2"), getBoxRect)).toBe("one-time-code");
  });

  it("offers addresses on the text fields of an address form, and logins still win on a login form", () => {
    document.body.innerHTML = addressAutocompletePage;
    expect(findInlineMenuPurpose(document, getInput("#ship-line1"), wideRect)).toBe("address");
    expect(findInlineMenuPurpose(document, getInput("#ship-postal"), wideRect)).toBe("address");
    expect(findInlineMenuPurpose(document, getInput("#ship-email"), wideRect)).toBeUndefined();
    expect(findInlineMenuPurpose(document, document.querySelector("#bill-street")!, wideRect)).toBeUndefined();
    document.body.innerHTML = checkoutLoginMixedPage;
    expect(findInlineMenuPurpose(document, getInput("#login-email"), wideRect)).toBe("login");
    expect(findInlineMenuPurpose(document, getInput("#city"), wideRect)).toBe("address");
    expect(findInlineMenuPurpose(document, getInput("#promo"), wideRect)).toBeUndefined();
  });

  it("ignores elements that are not inputs", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(findInlineMenuPurpose(document, document.querySelector("h1")!, wideRect)).toBeUndefined();
  });
});
