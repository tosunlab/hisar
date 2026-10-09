import { getAutocompleteTokens } from "./detection";
import { collectAllDeepElement, getDeepActiveElement, getParentAcrossShadow, isInputElement, isVisible } from "./dom";
import { setInputValue } from "./fill";

const allCodeType = new Set(["", "text", "tel", "number", "password"]);
const allOtherPurposeToken = new Set(["username", "email", "current-password", "new-password", "tel", "tel-national", "postal-code", "cc-number", "cc-csc"]);
const strongHintPattern =
  /\b(?:t?otp|[2t]fa|mfa|one ?time ?(?:code|pass ?(?:word|code)?|pin|token)|two ?factor|(?:2|two) ?step|authenticator|(?:authentication|verification|security|auth|sms|login|sign ?in|access) code)\b/;
const weakHintPattern = /\b(?:code|token|pin)\b/;
const excludedHintPattern =
  /\b(?:coupon|promo|discount|gift|voucher|zip|postal|post ?code|country|area|phone|mobile|captcha|search|referral|invite|invitation|sort ?code|swift|iban|card|cvc|cvv|csc)\b/;
const minimumSplitCount = 4;
const maximumSplitCount = 8;

function normalizeHint(text: string): string {
  return text
    .replace(/([a-z])([A-Z0-9])/g, "$1 $2")
    .replace(/([0-9])([a-zA-Z])/g, "$1 $2")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

function getNormalizedHints(input: HTMLInputElement): string {
  const allLabelText = Array.from(input.labels ?? []).map((label) => (label.textContent ?? "").slice(0, 80));
  const allHint = [input.name, input.id, input.getAttribute("placeholder"), input.getAttribute("aria-label"), input.getAttribute("title"), ...allLabelText];
  return allHint
    .filter((hint): hint is string => Boolean(hint))
    .map(normalizeHint)
    .join(" ")
    .replace(/\b2 fa\b/g, "2fa");
}

function isCodeInputCandidate(element: Element): element is HTMLInputElement {
  if (!isInputElement(element) || element.disabled) return false;
  const type = (element.getAttribute("type") ?? "").toLowerCase();
  if (!allCodeType.has(type)) return false;
  return isVisible(element);
}

function isNumericInput(input: HTMLInputElement): boolean {
  const type = (input.getAttribute("type") ?? "").toLowerCase();
  const pattern = input.getAttribute("pattern") ?? "";
  return type === "tel" || type === "number" || input.inputMode === "numeric" || input.getAttribute("inputmode") === "numeric" || /\\d|\[0-9\]/.test(pattern);
}

function hasCodeLength(input: HTMLInputElement): boolean {
  return input.maxLength >= 4 && input.maxLength <= 10;
}

function isOneTimeCodeAutocomplete(input: HTMLInputElement): boolean {
  return getAutocompleteTokens(input).includes("one-time-code");
}

function isSingleOneTimeCodeField(input: HTMLInputElement): boolean {
  if (isOneTimeCodeAutocomplete(input) && (input.maxLength < 0 || input.maxLength >= 4)) return true;
  if ((input.getAttribute("type") ?? "").toLowerCase() === "password") return false;
  if (getAutocompleteTokens(input).some((token) => allOtherPurposeToken.has(token))) return false;
  if (input.maxLength >= 0 && (input.maxLength < 4 || input.maxLength > 12)) return false;
  const hints = getNormalizedHints(input);
  if (!hints || excludedHintPattern.test(hints)) return false;
  if (strongHintPattern.test(hints)) return true;
  return weakHintPattern.test(hints) && (hasCodeLength(input) || isNumericInput(input));
}

function isSplitBox(element: Element): element is HTMLInputElement {
  return isCodeInputCandidate(element) && element.maxLength === 1;
}

function findSplitGroup(input: HTMLInputElement): HTMLInputElement[] | undefined {
  if (!isSplitBox(input)) return undefined;
  let container: Element | null = getParentAcrossShadow(input);
  for (let depth = 0; container && depth < 4; depth += 1) {
    const allBox = collectAllDeepElement(container, isSplitBox);
    if (allBox.length >= minimumSplitCount) {
      if (allBox.length > maximumSplitCount) return undefined;
      const allOtherField = collectAllDeepElement(container, isCodeInputCandidate).filter((candidate) => !allBox.includes(candidate));
      if (allOtherField.length > 0) return undefined;
      const isCodeGroup =
        allBox.length === 6 ||
        allBox.some(isOneTimeCodeAutocomplete) ||
        allBox.every(isNumericInput) ||
        allBox.some((box) => strongHintPattern.test(getNormalizedHints(box))) ||
        strongHintPattern.test(normalizeHint(container.getAttribute("aria-label") ?? ""));
      return isCodeGroup ? allBox : undefined;
    }
    if (container.tagName === "FORM" || container.tagName === "BODY") return undefined;
    container = getParentAcrossShadow(container);
  }
  return undefined;
}

// findOneTimeCodeTarget returns the field, or every box of a code split into one-character boxes, in order.
export function findOneTimeCodeTarget(element: Element): HTMLInputElement[] | undefined {
  if (!isCodeInputCandidate(element)) return undefined;
  const allBox = findSplitGroup(element);
  if (allBox) return allBox;
  return isSingleOneTimeCodeField(element) ? [element] : undefined;
}

export function isOneTimeCodeField(element: Element): element is HTMLInputElement {
  return findOneTimeCodeTarget(element) !== undefined;
}

export function findFirstOneTimeCodeTarget(document: Document): HTMLInputElement[] | undefined {
  const active = getDeepActiveElement(document);
  const focusedTarget = active ? findOneTimeCodeTarget(active) : undefined;
  if (focusedTarget) return focusedTarget;
  for (const input of collectAllDeepElement(document, isCodeInputCandidate)) {
    const target = findOneTimeCodeTarget(input);
    if (target) return target;
  }
  return undefined;
}

export function fillOneTimeCodeIntoTarget(allField: HTMLInputElement[], code: string): boolean {
  if (allField.length === 0 || !code || allField.some((field) => field.readOnly)) return false;
  if (allField.length === 1) {
    setInputValue(allField[0], code);
    return true;
  }
  const allCharacter = Array.from(code);
  if (allCharacter.length > allField.length) return false;
  allCharacter.forEach((character, index) => setInputValue(allField[index], character));
  allField[allCharacter.length - 1].focus();
  return true;
}
