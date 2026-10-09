import { afterEach, beforeEach, describe, expect, it } from "vitest";
import emailFirstStepOnePage from "./fixtures/email-first-step-one.html?raw";
import formlessLoginPage from "./fixtures/formless-login.html?raw";
import signUpPage from "./fixtures/sign-up.html?raw";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import { attachSubmissionCapture, type Submission } from "./capture";

let allSubmission: Submission[] = [];
let detach: () => void = () => undefined;
let currentTime = 0;

function getInput(selector: string): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

function startCapture(): void {
  detach = attachSubmissionCapture(document, { onSubmission: (submission) => allSubmission.push(submission), now: () => currentTime });
}

beforeEach(() => {
  allSubmission = [];
  currentTime = 0;
  document.body.innerHTML = "";
  document.addEventListener("submit", preventNavigation);
});

afterEach(() => {
  detach();
  document.removeEventListener("submit", preventNavigation);
});

function preventNavigation(event: Event): void {
  event.preventDefault();
}

describe("attachSubmissionCapture", () => {
  it("captures a login form on submit", () => {
    document.body.innerHTML = simpleLoginPage;
    startCapture();
    getInput("#login_field").value = "alex";
    getInput("#password").value = "secret";
    document.querySelector<HTMLFormElement>("#login")!.requestSubmit();
    expect(allSubmission).toEqual([{ kind: "login", username: "alex", password: "secret" }]);
  });

  it("captures on Enter in the password field", () => {
    document.body.innerHTML = simpleLoginPage;
    startCapture();
    getInput("#login_field").value = "alex";
    getInput("#password").value = "secret";
    getInput("#password").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, composed: true }));
    expect(allSubmission).toEqual([{ kind: "login", username: "alex", password: "secret" }]);
  });

  it("captures the field values at the time of a click on a script-driven sign-in button", () => {
    document.body.innerHTML = formlessLoginPage;
    startCapture();
    const [emailField] = Array.from(document.querySelectorAll<HTMLInputElement>(".login-panel input.input"));
    emailField.value = "alex@company.com";
    document.querySelector<HTMLInputElement>(".login-panel input[type=password]")!.value = "secret";
    document.getElementById("log-in")!.click();
    emailField.value = "";
    expect(allSubmission).toEqual([{ kind: "login", username: "alex@company.com", password: "secret" }]);
  });

  it("does not capture clicks on buttons that do not submit", () => {
    document.body.innerHTML = formlessLoginPage;
    startCapture();
    document.querySelector<HTMLInputElement>(".login-panel input[type=password]")!.value = "secret";
    document.querySelector<HTMLButtonElement>(".toggle")!.click();
    expect(allSubmission).toEqual([]);
  });

  it("reports one submission when a click is followed by the form submit", () => {
    document.body.innerHTML = simpleLoginPage;
    startCapture();
    getInput("#login_field").value = "alex";
    getInput("#password").value = "secret";
    getInput("#sign-in").click();
    currentTime = 100;
    document.querySelector<HTMLFormElement>("#login")!.requestSubmit();
    expect(allSubmission).toHaveLength(1);
  });

  it("reports the same values again after the duplicate window", () => {
    document.body.innerHTML = simpleLoginPage;
    startCapture();
    getInput("#login_field").value = "alex";
    getInput("#password").value = "secret";
    document.querySelector<HTMLFormElement>("#login")!.requestSubmit();
    currentTime = 5000;
    document.querySelector<HTMLFormElement>("#login")!.requestSubmit();
    expect(allSubmission).toHaveLength(2);
  });

  it("ignores a submit while the password is empty", () => {
    document.body.innerHTML = simpleLoginPage;
    startCapture();
    getInput("#login_field").value = "alex";
    document.querySelector<HTMLFormElement>("#login")!.requestSubmit();
    expect(allSubmission).toEqual([]);
  });

  it("reports the username of the first step of an email-first sign-in", () => {
    document.body.innerHTML = emailFirstStepOnePage;
    startCapture();
    getInput("#identifierId").value = "alex@example.com";
    document.getElementById("next")!.querySelector("span")!.click();
    expect(allSubmission).toEqual([{ kind: "username", username: "alex@example.com" }]);
  });

  it("captures the new password of a sign-up form", () => {
    document.body.innerHTML = signUpPage;
    startCapture();
    getInput("[name=email]").value = "new@example.com";
    getInput("#new-password").value = "fresh";
    getInput("#confirm-password").value = "fresh";
    document.querySelector<HTMLFormElement>("#sign-up")!.requestSubmit();
    expect(allSubmission).toEqual([{ kind: "login", username: "new@example.com", password: "fresh" }]);
  });

  it("captures logins inside open shadow roots on click", () => {
    const host = document.createElement("login-widget");
    document.body.append(host);
    const shadowRoot = host.attachShadow({ mode: "open" });
    shadowRoot.innerHTML = `<div><input type="email" id="e" /><input type="password" id="p" /><button type="button">Sign in</button></div>`;
    startCapture();
    (shadowRoot.getElementById("e") as HTMLInputElement).value = "alex";
    (shadowRoot.getElementById("p") as HTMLInputElement).value = "secret";
    shadowRoot.querySelector("button")!.click();
    expect(allSubmission).toEqual([{ kind: "login", username: "alex", password: "secret" }]);
  });
});
