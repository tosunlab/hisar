import { beforeEach, describe, expect, it } from "vitest";
import changePasswordPage from "./fixtures/change-password.html?raw";
import emailFirstStepOnePage from "./fixtures/email-first-step-one.html?raw";
import emailFirstStepTwoPage from "./fixtures/email-first-step-two.html?raw";
import formlessLoginPage from "./fixtures/formless-login.html?raw";
import modalAndPageLoginPage from "./fixtures/modal-and-page-login.html?raw";
import signUpPage from "./fixtures/sign-up.html?raw";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import unhintedLoginPage from "./fixtures/unhinted-login.html?raw";
import { captureLogin, findAllLoginScope, findUsernameField, isSubmitLikeButton, pickSubmittedPassword } from "./detection";

function loadPage(html: string): void {
  document.body.innerHTML = html;
}

function getInput(selector: string): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

beforeEach(() => {
  document.body.innerHTML = "";
  document.head.innerHTML = "";
});

describe("findAllLoginScope", () => {
  it("finds a simple login form and its username field, ignoring search and newsletter forms", () => {
    loadPage(simpleLoginPage);
    const allScope = findAllLoginScope(document);
    const login = allScope.find((scope) => scope.kind === "login");
    expect(login?.root).toBe(document.getElementById("login"));
    expect(login?.usernameField).toBe(getInput("#login_field"));
    expect(login?.allPasswordField).toEqual([getInput("#password")]);
    expect(allScope.some((scope) => scope.root.getAttribute("role") === "search")).toBe(false);
    expect(allScope.some((scope) => scope.root.getAttribute("action") === "/search-legacy")).toBe(false);
    expect(allScope.filter((scope) => scope.kind === "login")).toHaveLength(1);
  });

  it("treats the first step of an email-first sign-in as a username-only form", () => {
    loadPage(emailFirstStepOnePage);
    const allScope = findAllLoginScope(document);
    expect(allScope).toHaveLength(1);
    expect(allScope[0].kind).toBe("username-only");
    expect(allScope[0].usernameField).toBe(getInput("#identifierId"));
  });

  it("finds the password step of an email-first sign-in without a visible username", () => {
    loadPage(emailFirstStepTwoPage);
    const [scope] = findAllLoginScope(document);
    expect(scope.kind).toBe("login");
    expect(scope.usernameField).toBeUndefined();
    expect(scope.allPasswordField).toHaveLength(1);
  });

  it("groups fields without a form element by their nearest container with a submit-like control", () => {
    loadPage(formlessLoginPage);
    const allScope = findAllLoginScope(document);
    const login = allScope.find((scope) => scope.kind === "login");
    expect(login?.root).toBe(document.querySelector(".login-panel"));
    expect(login?.usernameField?.placeholder).toBe("you@company.com");
  });

  it("finds inputs inside open shadow roots", () => {
    const host = document.createElement("login-widget");
    document.body.append(host);
    const shadowRoot = host.attachShadow({ mode: "open" });
    shadowRoot.innerHTML = `<form id="shadow-form"><input type="email" id="shadow-email" /><input type="password" id="shadow-password" /><button>Sign in</button></form>`;
    const [scope] = findAllLoginScope(document);
    expect(scope.kind).toBe("login");
    expect(scope.usernameField).toBe(shadowRoot.getElementById("shadow-email"));
    expect(scope.allPasswordField[0]).toBe(shadowRoot.getElementById("shadow-password"));
  });

  it("classifies a form with a password and a confirmation as sign-up, not login", () => {
    loadPage(signUpPage);
    const [scope] = findAllLoginScope(document);
    expect(scope.kind).toBe("sign-up");
    expect(scope.allPasswordField).toHaveLength(2);
    expect(scope.usernameField?.name).toBe("email");
  });

  it("classifies a single new-password field as sign-up", () => {
    loadPage(`<form><input type="email" name="email" /><input type="password" autocomplete="new-password" /></form>`);
    expect(findAllLoginScope(document)[0].kind).toBe("sign-up");
  });

  it("classifies three password fields as a change-password form and keeps a read-only username", () => {
    loadPage(changePasswordPage);
    const [scope] = findAllLoginScope(document);
    expect(scope.kind).toBe("change-password");
    expect(scope.usernameField?.value).toBe("alex@example.com");
  });

  it("uses the text input just before the password when no field has hints, and skips hidden fields", () => {
    loadPage(unhintedLoginPage);
    const allScope = findAllLoginScope(document);
    expect(allScope).toHaveLength(1);
    expect(allScope[0].kind).toBe("login");
    expect(allScope[0].allPasswordField.map((field) => field.name)).toEqual(["f2"]);
    expect(allScope[0].usernameField?.name).toBe("f1");
  });

  it("finds two separate login forms on one page", () => {
    loadPage(modalAndPageLoginPage);
    const allScope = findAllLoginScope(document);
    expect(allScope.map((scope) => scope.root.id)).toEqual(["page-login", "modal-login"]);
    expect(allScope.map((scope) => scope.usernameField?.id)).toEqual(["page-user", "modal-email"]);
  });

  it("ignores disabled password fields and pages without logins", () => {
    loadPage(`<form><input type="text" name="q" /><input type="password" disabled /></form><p>Hello</p>`);
    expect(findAllLoginScope(document).filter((scope) => scope.kind !== "username-only")).toEqual([]);
  });

  it("does not treat a one-time-code input as the username", () => {
    loadPage(`<form><input type="password" id="p" /><input type="text" autocomplete="one-time-code" /></form>`);
    expect(findAllLoginScope(document)[0].usernameField).toBeUndefined();
  });
});

describe("findUsernameField", () => {
  it("prefers autocomplete=username over a nearer field", () => {
    loadPage(`<form id="f"><input id="a" autocomplete="username" /><input id="b" name="nickname" /><input type="password" id="p" /></form>`);
    expect(findUsernameField(document.getElementById("f")!, getInput("#p"))?.id).toBe("a");
  });

  it("prefers type=email and name hints over unrelated inputs", () => {
    loadPage(`<form id="f"><input id="a" name="company" /><input id="b" type="email" /><input type="password" id="p" /><input id="c" name="captcha" /></form>`);
    expect(findUsernameField(document.getElementById("f")!, getInput("#p"))?.id).toBe("b");
  });
});

describe("isSubmitLikeButton", () => {
  it("recognizes submit buttons, image inputs and sign-in buttons by label", () => {
    loadPage(`
      <button id="a">Go</button>
      <button id="b" type="button">Log in</button>
      <button id="c" type="button" aria-label="Show password">x</button>
      <input id="d" type="submit" value="Send" />
      <div id="e" role="button">Sign in</div>
      <div id="f">Sign in</div>
      <a id="g" href="#">Continue</a>`);
    const allResult = Object.fromEntries(
      ["a", "b", "c", "d", "e", "f", "g"].map((id) => [id, isSubmitLikeButton(document.getElementById(id)!)]),
    );
    expect(allResult).toEqual({ a: true, b: true, c: false, d: true, e: true, f: false, g: true });
  });
});

describe("captureLogin", () => {
  it("reads the username and password of a login form", () => {
    loadPage(simpleLoginPage);
    getInput("#login_field").value = " alex ";
    getInput("#password").value = "secret";
    const scope = findAllLoginScope(document).find((candidate) => candidate.kind === "login")!;
    expect(captureLogin(scope)).toEqual({ username: "alex", password: "secret" });
  });

  it("returns nothing while the password is empty", () => {
    loadPage(simpleLoginPage);
    getInput("#login_field").value = "alex";
    const scope = findAllLoginScope(document).find((candidate) => candidate.kind === "login")!;
    expect(captureLogin(scope)).toBeUndefined();
  });

  it("uses the new password of a sign-up form", () => {
    loadPage(signUpPage);
    document.querySelector<HTMLInputElement>("[name=email]")!.value = "new@example.com";
    getInput("#new-password").value = "fresh-password";
    getInput("#confirm-password").value = "fresh-password";
    const [scope] = findAllLoginScope(document);
    expect(captureLogin(scope)).toEqual({ username: "new@example.com", password: "fresh-password" });
  });

  it("uses the new password, not the current one, of a change-password form", () => {
    loadPage(changePasswordPage);
    getInput("#current").value = "old";
    getInput("#next-password").value = "new-one";
    getInput("#confirm").value = "new-one";
    const [scope] = findAllLoginScope(document);
    expect(pickSubmittedPassword(scope)).toBe("new-one");
  });

  it("uses the new-password field when autocomplete marks it", () => {
    loadPage(`<form><input type="password" autocomplete="current-password" id="a" /><input type="password" autocomplete="new-password" id="b" /></form>`);
    getInput("#a").value = "old";
    getInput("#b").value = "new";
    const [scope] = findAllLoginScope(document);
    expect(scope.kind).toBe("change-password");
    expect(pickSubmittedPassword(scope)).toBe("new");
  });
});
