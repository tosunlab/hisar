import { collectAllDeepElement, containsAcrossShadow, getParentAcrossShadow, isOffPage, isTransparent, isVisible } from "./dom";

// CardKind is what a payment field holds: the whole expiry, or its month or year alone.
export type CardKind = "number" | "name" | "expiry" | "expiry-month" | "expiry-year" | "security-code";

export type CardElement = HTMLInputElement | HTMLSelectElement;

export interface CardField {
  element: CardElement;
  kind: CardKind;
  isByAutocomplete: boolean;
}

export interface CardScope {
  root: Element;
  allField: CardField[];
}

const allAutocompleteKind: Record<string, CardKind> = {
  "cc-number": "number",
  "cc-name": "name",
  "cc-exp": "expiry",
  "cc-exp-month": "expiry-month",
  "cc-exp-year": "expiry-year",
  "cc-csc": "security-code",
};

const allCardInputType = new Set(["", "text", "tel", "number", "month", "password"]);

// The hint patterns, tried in this order: the security code and the expiry before the number, whose hints ("card") the
// others share, and a combined expiry ("MM / YY", "Expiration date") before its month and year.
const allHintKind: [CardKind, RegExp][] = [
  ["expiry", /mm[\s_/-]*yy|exp[a-z]*[\s_-]*date|valid[\s_-]*(thru|through|until)|son[\s_-]*kullanma|vencimiento/i],
  ["security-code", /\bcvv2?\b|\bcvc2?\b|\bcsc\b|\bcid\b|security[\s_-]*code|card[\s_-]*verification|verification[\s_-]*(code|number)|g[uü]venlik[\s_-]*kodu|pr[uü]fnummer|cryptogramme/i],
  ["expiry-month", /exp[a-z]*[\s_-]*(month|mm\b|mon\b)|card[\s_-]*month|cc[\s_-]*month|\bmonth\b|\bmm\b|monat|\bmois\b|\bmes\b|\bay\b/i],
  ["expiry-year", /exp[a-z]*[\s_-]*(year|yy)|card[\s_-]*year|cc[\s_-]*year|\byear\b|\byy(yy)?\b|\bjahr\b|ann[eé]e|\ba[nñ]o\b|\byıl\b/i],
  ["expiry", /expir|\bexp\b|ablauf|g[uü]ltig|date[\s_-]*d'?expiration/i],
  ["name", /name[\s_-]*on[\s_-]*card|card[\s_-]*holder|cardholder|holder[\s_-]*name|cc[\s_-]*name|card[\s_-]*name|name[\s_-]*on[\s_-]*the[\s_-]*card|kart[\s_-]*sahibi|karteninhaber|titulaire|titular/i],
  ["number", /card[\s_-]*(number|no\b|num\b|#)|cc[\s_-]*(number|num\b|no\b)|cardnumber|ccnum|credit[\s_-]*card|debit[\s_-]*card|kart[\s_-]*(numaras[ıi]|no)|kartennummer|num[eé]ro[\s_-]*de[\s_-]*(la[\s_-]*)?carte|n[uú]mero[\s_-]*de[\s_-]*(la[\s_-]*)?tarjeta|\bpan\b/i],
];

// A month or year field counts only next to a card number, since "month" alone is as likely a birth date.
const allExpiryPartKind = new Set<CardKind>(["expiry-month", "expiry-year"]);

const excludedHintPattern = /e-?mail|phone|\btel\b|search|coupon|promo|gift|captcha|birth|\bdob\b|\bzip\b|postal/i;

function getAutocompleteTokens(element: CardElement): string[] {
  return (element.getAttribute("autocomplete") ?? "").toLowerCase().split(/\s+/).filter(Boolean);
}

function getLabelText(element: CardElement): string {
  return Array.from(element.labels ?? [])
    .map((label) => label.textContent ?? "")
    .join(" ");
}

function getHints(element: CardElement): string {
  return [element.name, element.id, element.getAttribute("placeholder"), element.getAttribute("aria-label"), element.getAttribute("data-stripe"), getLabelText(element)]
    .filter((hint): hint is string => Boolean(hint))
    .join(" ");
}

function isCardElement(element: Element): element is CardElement {
  if (element.tagName === "SELECT") return true;
  return element.tagName === "INPUT" && allCardInputType.has((element.getAttribute("type") ?? "").toLowerCase());
}

// isFillable is a field Hisar may fill: shown, enabled and writable. A card fill writes every empty field of the form, so a
// transparent field, or one moved off the page, counts as hidden: a page cannot collect a number the user does not see.
function isFillable(element: CardElement): boolean {
  if (element.disabled || ("readOnly" in element && element.readOnly)) return false;
  return isVisible(element) && !isTransparent(element) && !isOffPage(element);
}

// classifyCardField says what a field of a payment form takes, by its autocomplete attribute or else its name, id,
// placeholder, label and aria-label, or returns undefined for any other field.
export function classifyCardField(element: Element): CardField | undefined {
  if (!isCardElement(element) || !isFillable(element)) return undefined;
  const allToken = getAutocompleteTokens(element);
  const autocompleteKind = allToken.map((token) => allAutocompleteKind[token]).find((kind) => kind !== undefined);
  if (autocompleteKind) return { element, kind: autocompleteKind, isByAutocomplete: true };
  if (allToken.some((token) => token !== "on" && token !== "off" && !token.startsWith("section-") && token !== "billing")) return undefined;
  const hints = getHints(element);
  if (!hints || excludedHintPattern.test(hints)) return undefined;
  // A list holds a month or a year, never a whole expiry or a number.
  const allCandidateKind = element.tagName === "SELECT" ? allHintKind.filter(([kind]) => allExpiryPartKind.has(kind)) : allHintKind;
  const kind = allCandidateKind.find(([, pattern]) => pattern.test(hints))?.[0];
  if (!kind) return undefined;
  if ((element as HTMLInputElement).type === "password" && kind !== "security-code") return undefined;
  return { element, kind, isByAutocomplete: false };
}

const maximumFormlessDepth = 8;

// findFormlessRoot is the nearest container of a field outside any form that holds another payment field, or else the
// body.
function findFormlessRoot(field: CardField, allFormless: CardField[]): Element {
  let current = getParentAcrossShadow(field.element);
  for (let depth = 0; current && depth < maximumFormlessDepth; depth += 1) {
    const ancestor = current;
    if (allFormless.some((other) => other !== field && containsAcrossShadow(ancestor, other.element))) return ancestor;
    if (ancestor.tagName === "BODY") break;
    current = getParentAcrossShadow(ancestor);
  }
  return field.element.ownerDocument.body;
}

// isCardScope holds a payment form: a card number field, so a lone "Name" or "Month" field is not one. A form whose
// number is in a payment provider's frame, with only the name here, is not filled from this frame.
function isCardScope(allField: CardField[]): boolean {
  return allField.some((field) => field.kind === "number");
}

// findAllCardScope groups the page's payment fields by their form, or for fields outside any form by the container they
// share; a month or year field is kept only in a scope with a card number.
export function findAllCardScope(document: Document): CardScope[] {
  const allField = collectAllDeepElement(document, isCardElement)
    .map(classifyCardField)
    .filter((field): field is CardField => field !== undefined);
  const allFormless = allField.filter((field) => !field.element.form);
  const formlessRoots = new Map(allFormless.map((field) => [field, findFormlessRoot(field, allFormless)]));
  const allScope: CardScope[] = [];
  for (const field of allField) {
    const root = field.element.form ?? formlessRoots.get(field) ?? document.body;
    const scope = allScope.find((candidate) => candidate.root === root || containsAcrossShadow(candidate.root, root) || containsAcrossShadow(root, candidate.root));
    if (scope) {
      scope.allField.push(field);
      if (containsAcrossShadow(root, scope.root)) scope.root = root;
    } else {
      allScope.push({ root, allField: [field] });
    }
  }
  return allScope.filter((scope) => isCardScope(scope.allField));
}

// findCardScopeOf returns the payment form that holds element, or undefined when it is not a payment field.
export function findCardScopeOf(document: Document, element: Element): CardScope | undefined {
  return findAllCardScope(document).find((scope) => scope.allField.some((field) => field.element === element));
}
