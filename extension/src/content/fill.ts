import type { FillOutcome, FrameProbe } from "../shared/messages";
import { findAllLoginScope, getAutocompleteTokens, isTextField, type LoginScope } from "./detection";
import { containsAcrossShadow, getDeepActiveElement, isInputElement } from "./dom";

export function isFrameFillable(window: Window, expectedOrigin: string): boolean {
  if (window.location.origin !== expectedOrigin || expectedOrigin === "null") return false;
  const allAncestorOrigin = Array.from(window.location.ancestorOrigins ?? []);
  return allAncestorOrigin.every((origin) => origin === expectedOrigin);
}

export function setInputValue(input: HTMLInputElement, value: string): void {
  input.focus();
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(input, value);
  else input.value = value;
  input.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertReplacementText", data: value }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

function findFocusedScope(document: Document, allScope: LoginScope[]): LoginScope | undefined {
  const active = getDeepActiveElement(document);
  if (!active) return undefined;
  return allScope.find((scope) => containsAcrossShadow(scope.root, active));
}

export function chooseFillScope(document: Document): LoginScope | undefined {
  const allScope = findAllLoginScope(document).filter((scope) => scope.kind === "login" || scope.kind === "username-only");
  const focused = findFocusedScope(document, allScope);
  if (focused) return focused;
  return allScope.find((scope) => scope.kind === "login") ?? allScope[0];
}

export function fillLogin(document: Document, username: string, password: string): FillOutcome {
  const scope = chooseFillScope(document);
  const outcome: FillOutcome = { filledUsername: false, filledPassword: false };
  if (!scope) return outcome;
  if (scope.usernameField && username && !scope.usernameField.readOnly) {
    setInputValue(scope.usernameField, username);
    outcome.filledUsername = true;
  }
  const passwordField = scope.allPasswordField[0];
  if (passwordField && password) {
    setInputValue(passwordField, password);
    outcome.filledPassword = true;
  }
  return outcome;
}

function getFocusedField(document: Document): HTMLInputElement | undefined {
  const active = getDeepActiveElement(document);
  if (!active || !isInputElement(active)) return undefined;
  if (active.type === "password" || isTextField(active)) return active;
  return undefined;
}

export function fillPasswordIntoFocusedField(document: Document, password: string): boolean {
  const focused = getFocusedField(document);
  if (!focused) return false;
  setInputValue(focused, password);
  if (focused.type !== "password") return true;
  const scope = findAllLoginScope(document).find((candidate) => candidate.allPasswordField.includes(focused));
  if (!scope || scope.kind === "login") return true;
  for (const confirmField of scope.allPasswordField) {
    if (confirmField === focused || getAutocompleteTokens(confirmField).includes("current-password")) continue;
    if (scope.allPasswordField.indexOf(confirmField) < scope.allPasswordField.indexOf(focused)) continue;
    setInputValue(confirmField, password);
  }
  focused.focus();
  return true;
}

export function probeFrame(document: Document): FrameProbe {
  const allScope = findAllLoginScope(document);
  const focusedField = getFocusedField(document);
  const focusedScope = focusedField ? allScope.find((scope) => containsAcrossShadow(scope.root, focusedField)) : undefined;
  return {
    hasLoginForm: allScope.some((scope) => scope.kind === "login"),
    hasUsernameField: allScope.some((scope) => scope.kind === "username-only"),
    hasFocusedField: focusedField !== undefined,
    hasFocusedLoginField: focusedScope?.kind === "login" || focusedScope?.kind === "username-only",
  };
}
