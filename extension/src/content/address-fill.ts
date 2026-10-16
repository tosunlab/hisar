import type { AddressValues } from "../shared/protocol";
import { getCountryName, findCountryCode, listCountrySpellings, normalizeCountryText } from "../shared/countries";
import { findAllAddressScope, findAddressScopeOf, type AddressField, type AddressScope } from "./address-detection";
import { getDeepActiveElement } from "./dom";

function dispatchFillEvents(element: HTMLElement, value: string): void {
  element.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertReplacementText", data: value }));
  element.dispatchEvent(new Event("change", { bubbles: true }));
}

// setFieldValue sets a value through the element's own prototype setter, as setInputValue does, so frameworks that track
// the value see the change.
function setFieldValue(element: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  element.focus();
  const prototype = element.tagName === "TEXTAREA" ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const valueSetter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
  if (valueSetter) valueSetter.call(element, value);
  else element.value = value;
  dispatchFillEvents(element, value);
}

// selectOption chooses the option whose value or text is one of allSpelling, ignoring case and accents, and reports
// whether there was one.
function selectOption(select: HTMLSelectElement, allSpelling: string[]): boolean {
  const option = Array.from(select.options).find(
    (candidate) => !candidate.disabled && (allSpelling.includes(normalizeCountryText(candidate.value)) || allSpelling.includes(normalizeCountryText(candidate.text))),
  );
  if (!option) return false;
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(select, option.value);
  else select.value = option.value;
  dispatchFillEvents(select, option.value);
  return true;
}

// buildFieldValues gives each field its value. The street goes whole into a street-address field, on lines in a textarea
// and joined with commas in an input; address-line fields take a line each, the last of them the rest. A street field next
// to a line 2 field counts as line 1.
function buildFieldValues(allField: AddressField[], address: AddressValues): Map<AddressField, string> {
  const allLine = address.street
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const allKind = new Set(allField.map((field) => field.kind));
  const hasLineFields = allKind.has("line1") || allKind.has("line2") || allKind.has("line3");
  const lineCount = allKind.has("line3") ? 3 : allKind.has("line2") ? 2 : 1;
  const getLine = (index: number): string => (index === lineCount - 1 ? allLine.slice(index).join(", ") : (allLine[index] ?? ""));
  const values = new Map<AddressField, string>();
  for (const field of allField) {
    let value = "";
    switch (field.kind) {
      case "street":
        value = hasLineFields ? getLine(0) : allLine.join(field.element.tagName === "TEXTAREA" ? "\n" : ", ");
        break;
      case "line1":
        value = getLine(0);
        break;
      case "line2":
        value = lineCount >= 2 ? getLine(1) : "";
        break;
      case "line3":
        value = getLine(2);
        break;
      case "city":
        value = address.city;
        break;
      case "region":
        value = address.region;
        break;
      case "postal":
        value = address.postalCode;
        break;
      case "country":
        value = address.country;
        break;
    }
    values.set(field, value);
  }
  return values;
}

// writeCountry writes a country into a text field in the form its autocomplete token asks for: a code for "country", a
// name for "country-name", and as saved otherwise.
function writeCountry(field: AddressField, country: string): string {
  const allToken = (field.element.getAttribute("autocomplete") ?? "").toLowerCase().split(/\s+/);
  if (allToken.includes("country")) return findCountryCode(country) ?? country;
  if (allToken.includes("country-name")) return getCountryName(country);
  return country;
}

// fillAddressIntoScope fills the scope's fields in the anchor's group: empty text fields only, so nothing the user typed
// is replaced, and lists when one of their options matches. It returns how many fields it filled.
function fillAddressIntoScope(scope: AddressScope, anchor: AddressField | undefined, address: AddressValues): number {
  const group = (anchor ?? scope.allField[0]).group;
  const allField = scope.allField.filter((field) => field.group === group);
  let filledCount = 0;
  for (const [field, value] of buildFieldValues(allField, address)) {
    if (!value) continue;
    const element = field.element;
    if (element.tagName === "SELECT") {
      const allSpelling = field.kind === "country" ? listCountrySpellings(value) : [normalizeCountryText(value)];
      if (selectOption(element as HTMLSelectElement, allSpelling)) filledCount += 1;
      continue;
    }
    const textField = element as HTMLInputElement | HTMLTextAreaElement;
    if (textField.value.trim() !== "") continue;
    setFieldValue(textField, field.kind === "country" ? writeCountry(field, value) : value);
    filledCount += 1;
  }
  anchor?.element.focus();
  return filledCount;
}

// fillAddressIntoScopeOf fills the address form of the field the inline menu is anchored to.
export function fillAddressIntoScopeOf(document: Document, element: Element, address: AddressValues): number {
  const scope = findAddressScopeOf(document, element);
  if (!scope) return 0;
  return fillAddressIntoScope(
    scope,
    scope.allField.find((field) => field.element === element),
    address,
  );
}

// fillAddressIntoPage fills the address form with the focused field, or else the page's first one.
export function fillAddressIntoPage(document: Document, address: AddressValues): number {
  const active = getDeepActiveElement(document);
  const allScope = findAllAddressScope(document);
  const focused = active ? allScope.find((scope) => scope.allField.some((field) => field.element === active)) : undefined;
  const scope = focused ?? allScope[0];
  if (!scope) return 0;
  return fillAddressIntoScope(
    scope,
    scope.allField.find((field) => field.element === active),
    address,
  );
}

export function hasAddressForm(document: Document): boolean {
  return findAllAddressScope(document).length > 0;
}

export function isFocusedAddressField(document: Document): boolean {
  const active = getDeepActiveElement(document);
  return active !== null && findAddressScopeOf(document, active) !== undefined;
}
