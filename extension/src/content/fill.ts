import { readAncestorOrigins } from "../shared/frame-ancestry";
import type { FillOutcome, FrameProbe } from "../shared/messages";
import { hasAddressForm, isFocusedAddressField } from "./address-fill";
import { hasCardForm, isFocusedCardField } from "./card-fill";
import { hasIdentityForm, isFocusedIdentityField } from "./identity-fill";
import { describeFieldForDiagnostics, logFillDiagnostics } from "./diagnostics";
import { findAllLoginScope, getAutocompleteTokens, isTextField, scoreUsernameField, type LoginScope } from "./detection";
import { collectAllDeepElement, containsAcrossShadow, getDeepActiveElement, isInputElement } from "./dom";
import { findFirstOneTimeCodeTarget, fillOneTimeCodeIntoTarget, findOneTimeCodeTarget } from "./one-time-code";

export function isFrameFillable(window: Window, expectedOrigin: string): boolean {
  if (window.location.origin !== expectedOrigin || expectedOrigin === "null") return false;
  return readAncestorOrigins(window).every((origin) => origin === expectedOrigin);
}

function setNativeValue(input: HTMLInputElement, value: string): void {
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(input, value);
  else input.value = value;
}

function dispatchKey(input: HTMLInputElement, type: "keydown" | "keypress" | "keyup", key: string): void {
  input.dispatchEvent(new KeyboardEvent(type, { key, bubbles: true, composed: true, cancelable: true }));
}

// setInputValue writes the value as a paste would, with key events around it for pages that keep their own copy of a
// field's value from key events.
export function setInputValue(input: HTMLInputElement, value: string): void {
  input.focus();
  const lastKey = value.at(-1) ?? "";
  dispatchKey(input, "keydown", lastKey);
  setNativeValue(input, value);
  input.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertReplacementText", data: value }));
  dispatchKey(input, "keyup", lastKey);
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

// typeInputValue enters the value one character at a time, for pages that rewrite a pasted value.
function typeInputValue(input: HTMLInputElement, value: string): void {
  input.focus();
  setNativeValue(input, "");
  for (let index = 0; index < value.length; index += 1) {
    const key = value[index];
    dispatchKey(input, "keydown", key);
    dispatchKey(input, "keypress", key);
    setNativeValue(input, value.slice(0, index + 1));
    input.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertText", data: key }));
    dispatchKey(input, "keyup", key);
  }
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

function waitForTask(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

// waitForRender waits until the page has handled the fill's events and drawn once; requestAnimationFrame alone never
// fires in a background tab.
function waitForRender(window: Window): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, 100);
    window.requestAnimationFrame(() => {
      clearTimeout(timer);
      setTimeout(resolve, 0);
    });
  });
}

// isSameValue allows a page to trim a username or change its case, as some sign-in pages do with email addresses.
function isSameValue(fieldValue: string, value: string, isUsername: boolean): boolean {
  if (fieldValue === value) return true;
  return isUsername && fieldValue.trim().toLowerCase() === value.trim().toLowerCase();
}

interface FieldReference {
  field: HTMLInputElement;
  id: string;
  name: string;
  type: string;
}

function describeField(field: HTMLInputElement): FieldReference {
  return { field, id: field.id, name: field.name, type: field.type };
}

// resolveLiveField returns the field, or the one the page put in its place when it rendered the form again.
export function resolveLiveField(document: Document, reference: FieldReference): HTMLInputElement | undefined {
  if (reference.field.isConnected) return reference.field;
  if (!reference.id && !reference.name) return undefined;
  return collectAllDeepElement(document, isInputElement).find(
    (input) => input.type === reference.type && (reference.id ? input.id === reference.id : input.name === reference.name),
  );
}

// writeLoginField fills one field and reports whether it took the value. A field read-only until focused, a trick
// against autofill, is filled once its focus handler has run.
async function writeLoginField(document: Document, reference: FieldReference, value: string): Promise<boolean> {
  let field = resolveLiveField(document, reference);
  if (!field || field.disabled) return false;
  field.focus();
  if (field.readOnly) await waitForTask();
  field = resolveLiveField(document, reference);
  if (!field || field.readOnly || field.disabled) return false;
  setInputValue(field, value);
  return true;
}

function findFocusedScope(document: Document, allScope: LoginScope[]): LoginScope | undefined {
  const active = getDeepActiveElement(document);
  if (!active) return undefined;
  return allScope.find((scope) => containsAcrossShadow(scope.root, active));
}

export function chooseFillScope(document: Document): LoginScope | undefined {
  const allScope = findAllLoginScope(document).filter((scope) => scope.kind === "login" || scope.kind === "username-only");
  logFillDiagnostics("login forms found", { allScope: allScope.map((scope) => ({ kind: scope.kind, username: describeFieldForDiagnostics(scope.usernameField) })) });
  const focused = findFocusedScope(document, allScope);
  if (focused) return focused;
  const login = allScope.find((scope) => scope.kind === "login");
  if (login) return login;
  // Without a login form, the field most clearly asking for a username: the first step of a sign-in, not a newsletter.
  const score = (scope: LoginScope) => (scope.usernameField ? scoreUsernameField(scope.usernameField) : 0);
  return allScope.reduce<LoginScope | undefined>((best, scope) => (!best || score(scope) > score(best) ? scope : best), undefined);
}

export function fillLogin(document: Document, username: string, password: string): Promise<FillOutcome> {
  return fillLoginIntoScope(document, chooseFillScope(document), username, password);
}

export function findScopeOfField(document: Document, field: HTMLInputElement): LoginScope | undefined {
  return findAllLoginScope(document).find((scope) => scope.usernameField === field || scope.allPasswordField.includes(field));
}

export async function fillLoginIntoScopeOf(document: Document, field: HTMLInputElement, username: string, password: string): Promise<FillOutcome> {
  const scope = findScopeOfField(document, field);
  if (!scope || (scope.kind !== "login" && scope.kind !== "username-only")) return { filledUsername: false, filledPassword: false };
  return fillLoginIntoScope(document, scope, username, password);
}

// fillLoginIntoScope fills the username and the password, then checks after the page has handled the events that each
// field kept its value, and types it in once more where the page rewrote it. A field reports filled only when it holds
// the value.
async function fillLoginIntoScope(document: Document, scope: LoginScope | undefined, username: string, password: string): Promise<FillOutcome> {
  const outcome: FillOutcome = { filledUsername: false, filledPassword: false };
  if (!scope) return outcome;
  const allFill: { reference: FieldReference; value: string; key: "filledUsername" | "filledPassword" }[] = [];
  if (scope.usernameField && username) allFill.push({ reference: describeField(scope.usernameField), value: username, key: "filledUsername" });
  if (scope.allPasswordField[0] && password) allFill.push({ reference: describeField(scope.allPasswordField[0]), value: password, key: "filledPassword" });
  logFillDiagnostics("login fill", {
    scope: scope.kind,
    username: describeFieldForDiagnostics(scope.usernameField),
    password: describeFieldForDiagnostics(scope.allPasswordField[0]),
  });
  const allWritten = [];
  for (const fill of allFill) {
    if (await writeLoginField(document, fill.reference, fill.value)) allWritten.push(fill);
    else logFillDiagnostics("field not written", { field: describeFieldForDiagnostics(fill.reference.field), isConnected: fill.reference.field.isConnected, isReadOnly: fill.reference.field.readOnly });
  }
  if (allWritten.length === 0) return outcome;
  const window = document.defaultView;
  if (window) await waitForRender(window);
  for (const fill of allWritten) {
    const isKept = (field: HTMLInputElement | undefined) => field !== undefined && isSameValue(field.value, fill.value, fill.key === "filledUsername");
    let field = resolveLiveField(document, fill.reference);
    if (field && !isKept(field) && !field.readOnly && !field.disabled) {
      logFillDiagnostics("value not kept, typing it", { field: describeFieldForDiagnostics(field), isReplaced: field !== fill.reference.field });
      typeInputValue(field, fill.value);
      if (window) await waitForRender(window);
      field = resolveLiveField(document, fill.reference);
    }
    outcome[fill.key] = isKept(field);
  }
  logFillDiagnostics("login fill done", { ...outcome });
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
  fillPasswordIntoField(document, focused, password);
  return true;
}

export function fillPasswordIntoField(document: Document, focused: HTMLInputElement, password: string): void {
  setInputValue(focused, password);
  if (focused.type !== "password") return;
  const scope = findAllLoginScope(document).find((candidate) => candidate.allPasswordField.includes(focused));
  if (!scope || scope.kind === "login") return;
  for (const confirmField of scope.allPasswordField) {
    if (confirmField === focused || getAutocompleteTokens(confirmField).includes("current-password")) continue;
    if (scope.allPasswordField.indexOf(confirmField) < scope.allPasswordField.indexOf(focused)) continue;
    setInputValue(confirmField, password);
  }
  focused.focus();
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
    hasOneTimeCodeField: findFirstOneTimeCodeTarget(document) !== undefined,
    hasAddressForm: hasAddressForm(document),
    hasFocusedAddressField: isFocusedAddressField(document),
    hasCardForm: hasCardForm(document),
    hasFocusedCardField: isFocusedCardField(document),
    hasIdentityForm: hasIdentityForm(document),
    hasFocusedIdentityField: isFocusedIdentityField(document),
  };
}

export function fillOneTimeCodeIntoPage(document: Document, code: string): boolean {
  const target = findFirstOneTimeCodeTarget(document);
  return target !== undefined && fillOneTimeCodeIntoTarget(target, code);
}

export function fillOneTimeCodeIntoField(field: HTMLInputElement, code: string): boolean {
  const target = findOneTimeCodeTarget(field);
  return target !== undefined && fillOneTimeCodeIntoTarget(target, code);
}
