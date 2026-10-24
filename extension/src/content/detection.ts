import { collectAllDeepElement, containsAcrossShadow, getParentAcrossShadow, isInputElement, isVisible } from "./dom";

export type ScopeKind = "login" | "sign-up" | "change-password" | "username-only";

export interface LoginScope {
  root: Element;
  kind: ScopeKind;
  allPasswordField: HTMLInputElement[];
  usernameField?: HTMLInputElement;
}

const allTextType = new Set(["", "text", "email", "tel", "url"]);
const usernameHintPattern = /user|login|e-?mail|account|identifier|ident|benutzer|usuario|kullan|courriel|nutzer/i;
const excludedHintPattern = /search|captcha|otp|one-?time|coupon|promo|zip|postal|verification|code|query|newsletter|subscribe/i;
const submitTextPattern = /log\s*-?\s*in|sign\s*-?\s*in|sign\s*on|continue|next|submit|enter|anmelden|connexion|se connecter|entrar|iniciar|giri[sş]|войти|weiter|suivant/i;

export function getAutocompleteTokens(input: HTMLInputElement): string[] {
  return (input.getAttribute("autocomplete") ?? "").toLowerCase().split(/\s+/).filter(Boolean);
}

export function isPasswordField(element: Element): element is HTMLInputElement {
  return (
    isInputElement(element) && element.type === "password" && !element.disabled && !getAutocompleteTokens(element).includes("one-time-code") && isVisible(element)
  );
}

export function isTextField(element: Element): element is HTMLInputElement {
  if (!isInputElement(element)) return false;
  const type = (element.getAttribute("type") ?? "").toLowerCase();
  if (!allTextType.has(type)) return false;
  if (element.disabled) return false;
  return isVisible(element);
}

// A label longer than this is more likely a paragraph around the field than its name.
const maximumLabelLength = 80;

function getLabelHints(input: HTMLInputElement): string[] {
  const allLabelText = Array.from(input.labels ?? [], (label) => label.textContent ?? "");
  const labelledBy = input.getAttribute("aria-labelledby");
  if (labelledBy) {
    const root = input.getRootNode() as Document | ShadowRoot;
    for (const id of labelledBy.split(/\s+/)) allLabelText.push(root.getElementById?.(id)?.textContent ?? "");
  }
  return allLabelText.map((text) => text.trim()).filter((text) => text.length <= maximumLabelLength);
}

function getFieldHints(input: HTMLInputElement): string {
  return [input.name, input.id, input.getAttribute("placeholder"), input.getAttribute("aria-label"), input.getAttribute("title"), ...getLabelHints(input)]
    .filter((hint): hint is string => Boolean(hint))
    .join(" ");
}

function isNewPasswordField(input: HTMLInputElement): boolean {
  return getAutocompleteTokens(input).includes("new-password");
}

function isCurrentPasswordField(input: HTMLInputElement): boolean {
  return getAutocompleteTokens(input).includes("current-password");
}

export function scoreUsernameField(input: HTMLInputElement): number {
  const allToken = getAutocompleteTokens(input);
  if (allToken.includes("one-time-code")) return -100;
  let score = 0;
  if (allToken.includes("username")) score += 100;
  else if (allToken.includes("email")) score += 90;
  if (input.type === "email") score += 60;
  const hints = getFieldHints(input);
  if (usernameHintPattern.test(hints)) score += 40;
  if (excludedHintPattern.test(hints)) score -= 80;
  return score;
}

export function isStrongUsernameField(input: HTMLInputElement): boolean {
  return scoreUsernameField(input) >= 40;
}

function isBefore(first: Element, second: Element, allOrdered: Element[]): boolean {
  return allOrdered.indexOf(first) < allOrdered.indexOf(second);
}

export function findUsernameField(scope: Element, firstPasswordField?: HTMLInputElement): HTMLInputElement | undefined {
  const allOrdered = collectAllDeepElement(scope, (element): element is Element => true);
  const allCandidate = allOrdered.filter(isTextField);
  if (allCandidate.length === 0) return undefined;
  const allBefore = firstPasswordField
    ? allCandidate.filter((candidate) => isBefore(candidate, firstPasswordField, allOrdered))
    : allCandidate;
  const nearestBefore = allBefore[allBefore.length - 1];
  let best: HTMLInputElement | undefined;
  let bestScore = 0;
  for (const candidate of allCandidate) {
    let score = scoreUsernameField(candidate);
    if (candidate === nearestBefore) score += 30;
    if (firstPasswordField && !allBefore.includes(candidate)) score -= 20;
    if (score > bestScore) {
      best = candidate;
      bestScore = score;
    }
  }
  return best;
}

function isSubmitLikeElement(element: Element): boolean {
  const tagName = element.tagName;
  if (tagName === "BUTTON") return true;
  if (tagName === "INPUT") {
    const type = (element as HTMLInputElement).type;
    return type === "submit" || type === "image" || type === "button";
  }
  return element.getAttribute("role") === "button";
}

function findFormlessScope(field: HTMLInputElement): Element {
  let firstWithOtherInput: Element | undefined;
  let current: Element | null = getParentAcrossShadow(field);
  for (let depth = 0; current && depth < 8; depth += 1) {
    if (collectAllDeepElement(current, (element): element is Element => isSubmitLikeButton(element)).length > 0) return current;
    if (!firstWithOtherInput && collectAllDeepElement(current, isTextField).length > 0) firstWithOtherInput = current;
    if (current.tagName === "BODY") break;
    current = getParentAcrossShadow(current);
  }
  return firstWithOtherInput ?? field.ownerDocument.body;
}

export function getScopeRoot(field: HTMLInputElement): Element {
  return field.form ?? findFormlessScope(field);
}

function classifyPasswordFields(allPasswordField: HTMLInputElement[]): ScopeKind {
  if (allPasswordField.length === 1) return isNewPasswordField(allPasswordField[0]) ? "sign-up" : "login";
  if (allPasswordField.length >= 3) return "change-password";
  return allPasswordField.some(isCurrentPasswordField) ? "change-password" : "sign-up";
}

export function findAllLoginScope(document: Document): LoginScope[] {
  const allScope: LoginScope[] = [];
  const allPasswordField = collectAllDeepElement(document, isPasswordField);
  for (const passwordField of allPasswordField) {
    const root = getScopeRoot(passwordField);
    const existing = allScope.find((scope) => scope.root === root);
    if (existing) existing.allPasswordField.push(passwordField);
    else allScope.push({ root, kind: "login", allPasswordField: [passwordField] });
  }
  for (const scope of allScope) {
    scope.kind = classifyPasswordFields(scope.allPasswordField);
    scope.usernameField = findUsernameField(scope.root, scope.allPasswordField[0]);
  }
  const allForm = collectAllDeepElement(document, (element): element is HTMLFormElement => element.tagName === "FORM");
  for (const form of allForm) {
    if (allScope.some((scope) => scope.root === form)) continue;
    if (!isVisible(form) || form.getAttribute("role") === "search") continue;
    const allTextField = collectAllDeepElement(form, isTextField);
    if (allTextField.length === 0 || allTextField.length > 2) continue;
    const usernameField = allTextField.find(isStrongUsernameField);
    if (usernameField) allScope.push({ root: form, kind: "username-only", allPasswordField: [], usernameField });
  }
  // A username field outside any form, as on the first step of many sign-ins, counts even beside a form that also asks for
  // an email address, such as a newsletter's.
  const allLooseField = collectAllDeepElement(document, isTextField).filter(
    (field) => !field.form && getAutocompleteTokens(field).some((token) => token === "username" || token === "email"),
  );
  for (const field of allLooseField) {
    if (allScope.some((scope) => containsAcrossShadow(scope.root, field))) continue;
    allScope.push({ root: findFormlessScope(field), kind: "username-only", allPasswordField: [], usernameField: field });
  }
  return allScope;
}

export function findScopeForElement(document: Document, element: Element): LoginScope | undefined {
  const allScope = findAllLoginScope(document);
  return allScope.find((scope) => containsAcrossShadow(scope.root, element) || scope.root === element);
}

export function isSubmitLikeButton(element: Element): boolean {
  if (element.tagName === "BUTTON") {
    const type = (element.getAttribute("type") ?? "submit").toLowerCase();
    if (type === "submit") return true;
  }
  if (isInputElement(element) && (element.type === "submit" || element.type === "image")) return true;
  if (!isSubmitLikeElement(element) && element.tagName !== "A") return false;
  const label = [element.textContent, element.getAttribute("aria-label"), (element as HTMLInputElement).value, element.id]
    .filter((text): text is string => Boolean(text))
    .join(" ")
    .trim();
  return label.length > 0 && label.length < 60 && submitTextPattern.test(label);
}

export interface CapturedLogin {
  username: string;
  password: string;
}

export function pickSubmittedPassword(scope: LoginScope): string {
  const allFilled = scope.allPasswordField.filter((field) => field.value);
  if (allFilled.length === 0) return "";
  if (scope.kind === "login") return allFilled[0].value;
  const allNew = allFilled.filter(isNewPasswordField);
  if (allNew.length > 0) return allNew[allNew.length - 1].value;
  const allNotCurrent = allFilled.filter((field) => !isCurrentPasswordField(field));
  if (scope.kind === "change-password" && allNotCurrent.length === allFilled.length && allFilled.length >= 3) {
    return allFilled[1].value;
  }
  const allCandidate = allNotCurrent.length > 0 ? allNotCurrent : allFilled;
  return allCandidate[allCandidate.length - 1].value;
}

export function captureLogin(scope: LoginScope): CapturedLogin | undefined {
  const password = pickSubmittedPassword(scope);
  if (!password) return undefined;
  return { username: scope.usernameField?.value.trim() ?? "", password };
}
