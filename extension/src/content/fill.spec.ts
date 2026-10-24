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
  it("fills the username and password of the login form", async () => {
    document.body.innerHTML = simpleLoginPage;
    expect(await fillLogin(document, "alex", "secret")).toEqual({ filledUsername: true, filledPassword: true });
    expect(getInput("#login_field").value).toBe("alex");
    expect(getInput("#password").value).toBe("secret");
    expect(document.querySelector<HTMLInputElement>("[name=newsletter_email]")!.value).toBe("");
    expect(document.querySelector<HTMLInputElement>("[name=q]")!.value).toBe("");
  });

  it("fills the form that holds the focus when there are several", async () => {
    document.body.innerHTML = modalAndPageLoginPage;
    getInput("#modal-email").focus();
    await fillLogin(document, "alex", "secret");
    expect(getInput("#modal-email").value).toBe("alex");
    expect(getInput("#modal-pass").value).toBe("secret");
    expect(getInput("#page-user").value).toBe("");
  });

  it("fills only the username on the first step of an email-first sign-in", async () => {
    document.body.innerHTML = emailFirstStepOnePage;
    expect(await fillLogin(document, "alex@example.com", "secret")).toEqual({ filledUsername: true, filledPassword: false });
    expect(getInput("#identifierId").value).toBe("alex@example.com");
  });

  it("does not fill a sign-up form", async () => {
    document.body.innerHTML = signUpPage;
    expect(await fillLogin(document, "alex", "secret")).toEqual({ filledUsername: false, filledPassword: false });
    expect(getInput("#new-password").value).toBe("");
  });

  it("does not overwrite a read-only username", async () => {
    document.body.innerHTML = `<form><input id="user" autocomplete="username" value="fixed" readonly /><input type="password" id="pass" /></form>`;
    expect(await fillLogin(document, "other", "secret")).toEqual({ filledUsername: false, filledPassword: true });
    expect(getInput("#user").value).toBe("fixed");
  });
  it("fills the first step of a sign-in without a form rather than a newsletter's email field", async () => {
    document.body.innerHTML = `<footer><form><input id="footer-email" type="email" name="email"><button>Send me offers</button></form></footer>
      <div><input id="user" type="email" autocomplete="username"><div role="button">Next</div></div>`;
    expect(await fillLogin(document, "alex@example.com", "secret")).toEqual({ filledUsername: true, filledPassword: false });
    expect(getInput("#user").value).toBe("alex@example.com");
    expect(getInput("#footer-email").value).toBe("");
  });

  it("fills the password field the page put in place of the one it had while the username was filled", async () => {
    document.body.innerHTML = `<form id="login"></form>`;
    const form = document.querySelector("form")!;
    const render = (username: string) => {
      form.innerHTML = `<input id="user" autocomplete="username"><input id="pass" type="password" autocomplete="current-password"><button>Sign in</button>`;
      getInput("#user").value = username;
      getInput("#user").addEventListener("input", (event) => render((event.target as HTMLInputElement).value));
    };
    render("");
    expect(await fillLogin(document, "alex", "secret")).toEqual({ filledUsername: true, filledPassword: true });
    expect(getInput("#user").value).toBe("alex");
    expect(getInput("#pass").value).toBe("secret");
  });

  it("fills a page that keeps its own copy of the value from key events", async () => {
    document.body.innerHTML = simpleLoginPage;
    let kept = "";
    getInput("#login_field").addEventListener("keyup", () => (kept = getInput("#login_field").value));
    getInput("#login_field").addEventListener("blur", () => (getInput("#login_field").value = kept));
    expect(await fillLogin(document, "alex", "secret")).toEqual({ filledUsername: true, filledPassword: true });
    expect(getInput("#login_field").value).toBe("alex");
  });

  it("fills a username that is read-only until focused", async () => {
    document.body.innerHTML = `<form><input id="user" autocomplete="username" readonly /><input type="password" id="pass" /></form>`;
    getInput("#user").addEventListener("focus", () => setTimeout(() => (getInput("#user").readOnly = false)));
    expect(await fillLogin(document, "alex", "secret")).toEqual({ filledUsername: true, filledPassword: true });
    expect(getInput("#user").value).toBe("alex");
  });

  it("reports a field that the page empties again as not filled, and accepts a username whose case the page changes", async () => {
    document.body.innerHTML = simpleLoginPage;
    getInput("#password").addEventListener("input", () => setTimeout(() => (getInput("#password").value = "")));
    getInput("#login_field").addEventListener("change", () => (getInput("#login_field").value = getInput("#login_field").value.toLowerCase()));
    expect(await fillLogin(document, "Alex", "secret")).toEqual({ filledUsername: true, filledPassword: false });
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

  it("refuses a frame inside another site in Firefox, which has no ancestorOrigins", () => {
    const parent = { location: { get origin(): string { throw new DOMException("Permission denied", "SecurityError"); } } } as unknown as Window;
    const frame = { location: { origin: "https://example.com" }, parent, top: parent } as unknown as Window;
    expect(isFrameFillable(frame, "https://example.com")).toBe(false);
  });

  it("refuses opaque origins", () => {
    expect(isFrameFillable(createFakeWindow("null", []), "null")).toBe(false);
  });
});

describe("probeFrame", () => {
  it("reports login forms and the focused field", () => {
    document.body.innerHTML = simpleLoginPage;
    expect(probeFrame(document)).toEqual({
      hasLoginForm: true,
      hasUsernameField: false,
      hasFocusedField: false,
      hasFocusedLoginField: false,
      hasOneTimeCodeField: false,
      hasAddressForm: false,
      hasFocusedAddressField: false,
      hasCardForm: false,
      hasFocusedCardField: false,
      hasIdentityForm: false,
      hasFocusedIdentityField: false,
    });
    getInput("#login_field").focus();
    expect(probeFrame(document)).toMatchObject({ hasFocusedField: true, hasFocusedLoginField: true });
  });

  it("reports a focused sign-up field as focused but not as a login field", () => {
    document.body.innerHTML = signUpPage;
    getInput("#new-password").focus();
    expect(probeFrame(document)).toEqual({
      hasLoginForm: false,
      hasUsernameField: false,
      hasFocusedField: true,
      hasFocusedLoginField: false,
      hasOneTimeCodeField: false,
      hasAddressForm: false,
      hasFocusedAddressField: false,
      hasCardForm: false,
      hasFocusedCardField: false,
      hasIdentityForm: true,
      hasFocusedIdentityField: false,
    });
  });
});
