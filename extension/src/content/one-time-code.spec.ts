import { beforeEach, describe, expect, it } from "vitest";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import twoFactorPage from "./fixtures/two-factor.html?raw";
import { findAllLoginScope } from "./detection";
import { fillOneTimeCodeIntoField, fillOneTimeCodeIntoPage, probeFrame } from "./fill";
import { findFirstOneTimeCodeTarget, findOneTimeCodeTarget, isOneTimeCodeField } from "./one-time-code";

function getInput(selector: string): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

function createSplitBoxes(count: number, attributes = 'maxlength="1" inputmode="numeric"'): string {
  return Array.from({ length: count }, (_, index) => `<input id="box-${index}" ${attributes} />`).join("");
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("isOneTimeCodeField", () => {
  it.each([
    ['<input autocomplete="one-time-code" />', true],
    ['<input type="tel" name="otp" />', true],
    ['<input name="app_totp" />', true],
    ['<input id="mfaCode" />', true],
    ['<input name="2fa_code" />', true],
    ['<input placeholder="Enter the 6-digit verification code" />', true],
    ['<input aria-label="Authentication code" />', true],
    ['<input name="code" maxlength="6" />', true],
    ['<input name="code" inputmode="numeric" />', true],
    ['<input type="number" name="token" />', true],
    ['<label for="field">Two-factor code</label><input id="field" />', true],
    ['<input name="code" />', false],
    ['<input name="promo_code" maxlength="6" />', false],
    ['<input name="postal_code" inputmode="numeric" />', false],
    ['<input name="otp" autocomplete="username" />', false],
    ['<input name="otp" maxlength="2" />', false],
    ['<input type="email" name="otp" />', false],
    ['<input type="password" name="otp" />', false],
    ['<input type="password" autocomplete="one-time-code" />', true],
    ['<input name="footprint" />', false],
    ['<input name="username" />', false],
    ['<input autocomplete="one-time-code" disabled />', false],
    ['<input autocomplete="one-time-code" hidden />', false],
  ])("%s is a code field: %s", (html, expected) => {
    document.body.innerHTML = html;
    expect(isOneTimeCodeField(document.querySelector("input")!)).toBe(expected);
  });
});

describe("split code boxes", () => {
  it("groups six one-character boxes in order", () => {
    document.body.innerHTML = `<form><div class="boxes">${createSplitBoxes(6, 'maxlength="1"')}</div><button>Verify</button></form>`;
    const target = findOneTimeCodeTarget(getInput("#box-3"));
    expect(target?.map((box) => box.id)).toEqual(["box-0", "box-1", "box-2", "box-3", "box-4", "box-5"]);
  });

  it("groups boxes that each sit in their own wrapper", () => {
    const allWrapped = Array.from({ length: 4 }, (_, index) => `<span><input id="box-${index}" maxlength="1" inputmode="numeric" /></span>`).join("");
    document.body.innerHTML = `<div aria-label="Security code">${allWrapped}</div>`;
    expect(findOneTimeCodeTarget(getInput("#box-0"))).toHaveLength(4);
  });

  it("does not group too few boxes, too many, or boxes next to other fields", () => {
    document.body.innerHTML = `<div>${createSplitBoxes(3)}</div>`;
    expect(findOneTimeCodeTarget(getInput("#box-0"))).toBeUndefined();
    document.body.innerHTML = `<div>${createSplitBoxes(9)}</div>`;
    expect(findOneTimeCodeTarget(getInput("#box-0"))).toBeUndefined();
    document.body.innerHTML = `<div><input id="name" />${createSplitBoxes(6)}</div>`;
    expect(findOneTimeCodeTarget(getInput("#box-0"))).toBeUndefined();
    document.body.innerHTML = `<div>${createSplitBoxes(5, 'maxlength="1"')}</div>`;
    expect(findOneTimeCodeTarget(getInput("#box-0"))).toBeUndefined();
  });

  it("types one character into each box and focuses the last", () => {
    document.body.innerHTML = `<div>${createSplitBoxes(6)}</div>`;
    const allInputEvent: string[] = [];
    document.addEventListener("input", (event) => allInputEvent.push((event.target as HTMLInputElement).id));
    expect(fillOneTimeCodeIntoField(getInput("#box-2"), "123456")).toBe(true);
    expect(Array.from(document.querySelectorAll("input"), (box) => box.value).join("")).toBe("123456");
    expect(allInputEvent).toEqual(["box-0", "box-1", "box-2", "box-3", "box-4", "box-5"]);
    expect(document.activeElement).toBe(getInput("#box-5"));
  });

  it("refuses a code longer than the boxes", () => {
    document.body.innerHTML = `<div>${createSplitBoxes(6)}</div>`;
    expect(fillOneTimeCodeIntoField(getInput("#box-0"), "12345678")).toBe(false);
    expect(getInput("#box-0").value).toBe("");
  });
});

describe("filling a code", () => {
  it("fills the two-factor page's field and leaves the login detection alone", () => {
    document.body.innerHTML = twoFactorPage;
    expect(findAllLoginScope(document)).toEqual([]);
    expect(probeFrame(document)).toMatchObject({ hasLoginForm: false, hasOneTimeCodeField: true });
    expect(fillOneTimeCodeIntoPage(document, "987654")).toBe(true);
    expect(getInput("#app_totp").value).toBe("987654");
  });

  it("prefers the focused code field", () => {
    document.body.innerHTML = `<input id="first" autocomplete="one-time-code" /><input id="second" autocomplete="one-time-code" />`;
    getInput("#second").focus();
    expect(findFirstOneTimeCodeTarget(document)).toEqual([getInput("#second")]);
  });

  it("finds no code field on a login page and refuses read-only fields", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(fillOneTimeCodeIntoPage(document, "123456")).toBe(false);
    document.body.innerHTML = `<input id="code" autocomplete="one-time-code" readonly />`;
    expect(fillOneTimeCodeIntoField(getInput("#code"), "123456")).toBe(false);
  });
});
