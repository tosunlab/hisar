import { getScopeRoot } from "./detection";
import { collectAllDeepElement, containsAcrossShadow, isVisible } from "./dom";

// AddressKind is what an address field holds. A street-address field takes the whole street; address-line fields take a
// line each.
export type AddressKind = "street" | "line1" | "line2" | "line3" | "city" | "region" | "postal" | "country";

export type AddressElement = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

export interface AddressField {
  element: AddressElement;
  kind: AddressKind;
  // group tells a shipping address from a billing one on the same form: the autocomplete section and shipping or billing
  // token, or "billing" or "shipping" in the field's hints; "" when it names neither.
  group: string;
  isByAutocomplete: boolean;
}

export interface AddressScope {
  root: Element;
  allField: AddressField[];
}

const allAutocompleteKind: Record<string, AddressKind> = {
  "street-address": "street",
  "address-line1": "line1",
  "address-line2": "line2",
  "address-line3": "line3",
  "address-level2": "city",
  "address-level1": "region",
  "postal-code": "postal",
  country: "country",
  "country-name": "country",
};

// Autocomplete tokens that name what a field is, so a field with another of them, such as email or cc-number, is not an
// address field whatever its hints say. "on" and "off" say nothing.
const allNonAddressToken = new Set([
  "name", "honorific-prefix", "given-name", "additional-name", "family-name", "honorific-suffix", "nickname", "username",
  "new-password", "current-password", "one-time-code", "organization-title", "organization", "address-level3",
  "address-level4", "cc-name", "cc-given-name", "cc-additional-name", "cc-family-name", "cc-number", "cc-exp", "cc-exp-month",
  "cc-exp-year", "cc-csc", "cc-type", "transaction-currency", "transaction-amount", "language", "bday", "bday-day",
  "bday-month", "bday-year", "sex", "url", "photo", "tel", "tel-country-code", "tel-national", "tel-area-code", "tel-local",
  "tel-extension", "email", "impp",
]);

const allAddressInputType = new Set(["", "text", "tel", "number"]);

// The hint patterns, tried in this order: a second line before a street, a postal code before a city.
const allHintKind: [AddressKind, RegExp][] = [
  ["line3", /address[\s_-]*(line)?[\s_-]*3|addr3|line[\s_-]*3/i],
  ["line2", /address[\s_-]*(line)?[\s_-]*2|addr2|line[\s_-]*2|apartment|\bapt\b|suite|\bunit\b|adres(s|se)?[\s_-]*2/i],
  ["line1", /address[\s_-]*(line)?[\s_-]*1|addr1|line[\s_-]*1/i],
  ["postal", /zip|postal|post[\s_-]*code|postcode|\bplz\b|posta[\s_-]*kodu|c[oó]digo[\s_-]*postal|code[\s_-]*postal|\bcp\b/i],
  ["city", /city|town|locality|stadt|\bort\b|ville|ciudad|[sş]ehir|il[cç]e/i],
  ["region", /\bstate\b|province|region|county|bundesland|provincia|prefecture/i],
  ["country", /country|\bland\b|\bpays\b|pa[ií]s|[uü]lke/i],
  ["street", /street|address|addr\b|adresse|adres|stra(ss|ß)e|calle|direcci[oó]n/i],
];

const excludedHintPattern = /e-?mail|phone|mobile|\btel\b|company|search|coupon|promo|captcha|password|card|\bweb|url|\bip\b|ip[\s_-]*address/i;

function getAutocompleteTokens(element: AddressElement): string[] {
  return (element.getAttribute("autocomplete") ?? "").toLowerCase().split(/\s+/).filter(Boolean);
}

function getLabelText(element: AddressElement): string {
  return Array.from(element.labels ?? [])
    .map((label) => label.textContent ?? "")
    .join(" ");
}

function getHints(element: AddressElement): string {
  return [element.name, element.id, element.getAttribute("placeholder"), element.getAttribute("aria-label"), getLabelText(element)]
    .filter((hint): hint is string => Boolean(hint))
    .join(" ");
}

function isAddressElement(element: Element): element is AddressElement {
  if (element.tagName === "SELECT" || element.tagName === "TEXTAREA") return true;
  return element.tagName === "INPUT" && allAddressInputType.has((element.getAttribute("type") ?? "").toLowerCase());
}

// isFillable is a field Hisar may fill: shown, enabled and writable. A hidden field is never filled, so a page cannot collect
// an address the user does not see.
function isFillable(element: AddressElement): boolean {
  if (element.disabled || ("readOnly" in element && element.readOnly)) return false;
  return isVisible(element);
}

// classifyAddressField says what address value a field takes, by its autocomplete attribute or else its name, id,
// placeholder, label and aria-label, or returns undefined for any other field.
export function classifyAddressField(element: Element): AddressField | undefined {
  if (!isAddressElement(element) || !isFillable(element)) return undefined;
  const allToken = getAutocompleteTokens(element);
  const groupTokens = allToken.filter((token) => token.startsWith("section-") || token === "shipping" || token === "billing").join(" ");
  const autocompleteKind = allToken.map((token) => allAutocompleteKind[token]).find((kind) => kind !== undefined);
  if (autocompleteKind) return { element, kind: autocompleteKind, group: groupTokens, isByAutocomplete: true };
  if (allToken.some((token) => allNonAddressToken.has(token))) return undefined;
  const hints = getHints(element);
  if (!hints || excludedHintPattern.test(hints)) return undefined;
  const hintKind = allHintKind.find(([, pattern]) => pattern.test(hints))?.[0];
  if (!hintKind) return undefined;
  // A street hint on a select is a list of saved addresses, not a street.
  if (element.tagName === "SELECT" && hintKind !== "country" && hintKind !== "region") return undefined;
  const group = groupTokens || (/bill/i.test(hints) ? "billing" : /ship|deliver/i.test(hints) ? "shipping" : "");
  return { element, kind: hintKind, group, isByAutocomplete: false };
}

// isAddressScope holds an address form: a field named by its autocomplete attribute, or two kinds of field found by their
// hints, so a lone "Address" field, such as an email address, is not one.
function isAddressScope(allField: AddressField[]): boolean {
  return allField.some((field) => field.isByAutocomplete) || new Set(allField.map((field) => field.kind)).size >= 2;
}

// findAllAddressScope groups the page's address fields by their form, or the container of a form without one.
export function findAllAddressScope(document: Document): AddressScope[] {
  const allScope: AddressScope[] = [];
  for (const element of collectAllDeepElement(document, isAddressElement)) {
    const field = classifyAddressField(element);
    if (!field) continue;
    const root = getScopeRoot(element);
    const scope = allScope.find((candidate) => candidate.root === root);
    if (scope) scope.allField.push(field);
    else allScope.push({ root, allField: [field] });
  }
  return allScope.filter((scope) => isAddressScope(scope.allField));
}

// findAddressScopeOf returns the address scope that holds element, or undefined when it is not an address field.
export function findAddressScopeOf(document: Document, element: Element): AddressScope | undefined {
  return findAllAddressScope(document).find((scope) => scope.allField.some((field) => field.element === element) && containsAcrossShadow(scope.root, element));
}
