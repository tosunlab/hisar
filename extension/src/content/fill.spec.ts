import { beforeEach, describe, expect, it, vi } from "vitest";
import emailFirstStepOnePage from "./fixtures/email-first-step-one.html?raw";
import modalAndPageLoginPage from "./fixtures/modal-and-page-login.html?raw";
import signUpPage from "./fixtures/sign-up.html?raw";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import { fillLogin, fillPasswordIntoFocusedField, isFrameFillable, probeFrame, setInputValue } from "./fill";

function getInput(selector: string): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

function createFakeWindow(origin: string, allAncestorOrigin?: string[]): Window {
  return { location: { origin, ancestorOrigins: allAncestorOrigin } } as unknown as Window;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("setInputValue", () => {
  it("dispatches bubbling input and change events after the value is set", () => {
    document.body.innerHTML = `<form><input id="field" /></form>`;
    const field = getInput("#field");
    const allEvent: string[] = [];
    document.addEventListener("input", (event) => allEvent.push(`${event.type}:${(event.target as HTMLInputElement).value}`));
    document.addEventListener("change", (event) => allEvent.push(`${event.type}:${(event.target as HTMLInputElement).value}`));
    setInputValue(field, "alex");
    expect(allEvent).toEqual(["input:alex", "change:alex"]);
    expect(document.activeElement).toBe(field);
  });

  it("bypasses an instance-level value setter the way React's value tracker needs", () => {
    document.body.innerHTML = `<input id="field" />`;
    const field = getInput("#field");
    const trackedSetter = vi.fn();
    const prototypeDescriptor = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!;
    Object.defineProperty(field, "value", {
      configurable: true,
      get: () => prototypeDescriptor.get!.call(field),
      set: trackedSetter,
    });
    let valueSeenByListener = "";
    field.addEventListener("input", () => (valueSeenByListener = prototypeDescriptor.get!.call(field)));
    setInputValue(field, "secret");
    expect(trackedSetter).not.toHaveBeenCalled();
    expect(valueSeenByListener).toBe("secret");
  });

  it("dispatches events that cross shadow boundaries", () => {
    const host = document.createElement("div");
    document.body.append(host);
    const shadowRoot = host.attachShadow({ mode: "open" });
    shadowRoot.innerHTML = `<input id="inner" />`;
    const listener = vi.fn();
    document.addEventListener("input", listener);
    setInputValue(shadowRoot.getElementById("inner") as HTMLInputElement, "x");
    expect(listener).toHaveBeenCalledTimes(1);
  });
});

describe("fillLogin", () => {
  it("fills the username and password of the login form", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(fillLogin(document, "alex", "secret")).toEqual({ filledUsername: true, filledPassword: true });
    expect(getInput("#login_field").value).toBe("alex");
    expect(getInput("#password").value).toBe("secret");
    expect(document.querySelector<HTMLInputElement>("[name=newsletter_email]")!.value).toBe("");
    expect(document.querySelector<HTMLInputElement>("[name=q]")!.value).toBe("");
  });

  it("fills the form that holds the focus when there are several", () => {
    document.body.innerHTML = modalAndPageLoginPage;
    getInput("#modal-email").focus();
    fillLogin(document, "alex", "secret");
    expect(getInput("#modal-email").value).toBe("alex");
    expect(getInput("#modal-pass").value).toBe("secret");
    expect(getInput("#page-user").value).toBe("");
  });

  it("fills only the username on the first step of an email-first sign-in", () => {
    document.body.innerHTML = emailFirstStepOnePage;
    expect(fillLogin(document, "alex@example.com", "secret")).toEqual({ filledUsername: true, filledPassword: false });
    expect(getInput("#identifierId").value).toBe("alex@example.com");
  });

  it("does not fill a sign-up form", () => {
    document.body.innerHTML = signUpPage;
    expect(fillLogin(document, "alex", "secret")).toEqual({ filledUsername: false, filledPassword: false });
    expect(getInput("#new-password").value).toBe("");
  });

  it("does not overwrite a read-only username", () => {
    document.body.innerHTML = `<form><input id="user" autocomplete="username" value="fixed" readonly /><input type="password" id="pass" /></form>`;
    expect(fillLogin(document, "other", "secret")).toEqual({ filledUsername: false, filledPassword: true });
    expect(getInput("#user").value).toBe("fixed");
  });
});

describe("fillPasswordIntoFocusedField", () => {
  it("fills the focused field and the confirmation field of a sign-up form", () => {
    document.body.innerHTML = signUpPage;
    getInput("#new-password").focus();
    expect(fillPasswordIntoFocusedField(document, "generated")).toBe(true);
    expect(getInput("#new-password").value).toBe("generated");
    expect(getInput("#confirm-password").value).toBe("generated");
  });

  it("fills only the focused field of a login form", () => {
    document.body.innerHTML = simpleLoginPage;
    getInput("#password").focus();
    expect(fillPasswordIntoFocusedField(document, "generated")).toBe(true);
    expect(getInput("#password").value).toBe("generated");
    expect(getInput("#login_field").value).toBe("");
  });

  it("refuses when nothing editable has the focus", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(fillPasswordIntoFocusedField(document, "generated")).toBe(false);
  });
});

describe("isFrameFillable (frame rule)", () => {
  it("allows the top frame with the expected origin", () => {
    expect(isFrameFillable(createFakeWindow("https://example.com", []), "https://example.com")).toBe(true);
  });

  it("allows a same-origin child frame", () => {
    expect(isFrameFillable(createFakeWindow("https://example.com", ["https://example.com"]), "https://example.com")).toBe(true);
  });

  it("refuses a frame whose origin differs from the expected one", () => {
    expect(isFrameFillable(createFakeWindow("https://evil.example", ["https://example.com"]), "https://example.com")).toBe(false);
  });

  it("refuses a same-origin frame nested inside a cross-origin frame", () => {
    expect(
      isFrameFillable(createFakeWindow("https://example.com", ["https://ads.example.net", "https://example.com"]), "https://example.com"),
    ).toBe(false);
  });

  it("refuses opaque origins", () => {
    expect(isFrameFillable(createFakeWindow("null", []), "null")).toBe(false);
  });
});

describe("probeFrame", () => {
  it("reports login forms and the focused field", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(probeFrame(document)).toEqual({ hasLoginForm: true, hasUsernameField: false, hasFocusedField: false, hasFocusedLoginField: false, hasOneTimeCodeField: false });
    getInput("#login_field").focus();
    expect(probeFrame(document)).toMatchObject({ hasFocusedField: true, hasFocusedLoginField: true });
  });

  it("reports a focused sign-up field as focused but not as a login field", () => {
    document.body.innerHTML = signUpPage;
    getInput("#new-password").focus();
    expect(probeFrame(document)).toEqual({ hasLoginForm: false, hasUsernameField: false, hasFocusedField: true, hasFocusedLoginField: false, hasOneTimeCodeField: false });
  });
});
