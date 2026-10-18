import type { CardValues } from "../shared/protocol";
import { findAllCardScope, findCardScopeOf, type CardField, type CardScope } from "./card-detection";
import { getDeepActiveElement } from "./dom";

const allMonthName = [
  ["jan", "january", "ocak", "januar", "janvier", "enero"],
  ["feb", "february", "şubat", "subat", "februar", "février", "fevrier", "febrero"],
  ["mar", "march", "mart", "märz", "marz", "mars", "marzo"],
  ["apr", "april", "nisan", "avril", "abril"],
  ["may", "mayıs", "mayis", "mai", "mayo"],
  ["jun", "june", "haziran", "juni", "juin", "junio"],
  ["jul", "july", "temmuz", "juli", "juillet", "julio"],
  ["aug", "august", "ağustos", "agustos", "août", "aout", "agosto"],
  ["sep", "sept", "september", "eylül", "eylul", "septembre", "septiembre"],
  ["oct", "october", "ekim", "oktober", "octobre", "octubre"],
  ["nov", "november", "kasım", "kasim", "novembre", "noviembre"],
  ["dec", "december", "aralık", "aralik", "dezember", "décembre", "decembre", "diciembre"],
];

function dispatchFillEvents(element: HTMLElement, value: string): void {
  element.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertReplacementText", data: value }));
  element.dispatchEvent(new Event("change", { bubbles: true }));
}

// setInputValue sets a value through the prototype setter, so frameworks that track the value see the change.
function setInputValue(input: HTMLInputElement, value: string): void {
  input.focus();
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(input, value);
  else input.value = value;
  dispatchFillEvents(input, value);
}

function selectValue(select: HTMLSelectElement, value: string): void {
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(select, value);
  else select.value = value;
  dispatchFillEvents(select, value);
}

// readOptionNumber is the number an option of a month or year list stands for: its value, else the number its text
// starts with ("03 - March"), else a month's name.
function readOptionNumber(option: HTMLOptionElement, isMonth: boolean): number | undefined {
  for (const text of [option.value, option.text]) {
    const match = text.trim().match(/^\d+/);
    if (match) return Number(match[0]);
  }
  if (!isMonth) return undefined;
  const text = option.text.trim().toLowerCase();
  const index = allMonthName.findIndex((allName) => allName.some((name) => text === name || text.startsWith(name)));
  return index >= 0 ? index + 1 : undefined;
}

// chooseOption selects the list's option for a month (1–12) or a year (2030 also matches a "30" option).
function chooseOption(select: HTMLSelectElement, number: number, isMonth: boolean): boolean {
  const option = Array.from(select.options).find((candidate) => {
    if (candidate.disabled) return false;
    const optionNumber = readOptionNumber(candidate, isMonth);
    return optionNumber === number || (!isMonth && optionNumber === number % 100);
  });
  if (!option) return false;
  selectValue(select, option.value);
  return true;
}

function getMaximumLength(input: HTMLInputElement): number {
  return input.maxLength > 0 ? input.maxLength : 0;
}

// formatExpiry writes the expiry as the field asks: YYYY-MM for a month input, MM/YYYY when the placeholder or length
// asks for four digits of year, else MM/YY; a field of four characters gets MMYY.
function formatExpiry(input: HTMLInputElement, month: string, year: string): string {
  if (input.type === "month") return `${year}-${month}`;
  const hints = `${input.placeholder} ${input.getAttribute("aria-label") ?? ""}`.toLowerCase();
  const maximumLength = getMaximumLength(input);
  if (/yyyy|jjjj|aaaa/.test(hints) || maximumLength === 7 || maximumLength === 9) return `${month}/${year}`;
  if (maximumLength === 4) return `${month}${year.slice(2)}`;
  const separator = /mm\s+\/\s+yy/.test(hints) ? " / " : /mm\s*-\s*yy/.test(hints) ? "-" : "/";
  return `${month}${separator}${year.slice(2)}`;
}

// fillNumber puts the number in one field, or across split boxes in order, each taking as many digits as it holds.
function fillNumber(allNumberField: CardField[], digits: string): number {
  const allInput = allNumberField.map((field) => field.element).filter((element): element is HTMLInputElement => element.tagName === "INPUT");
  if (allInput.length === 0 || allInput.some((input) => input.value.trim() !== "")) return 0;
  if (allInput.length === 1) {
    setInputValue(allInput[0], digits);
    return 1;
  }
  let rest = digits;
  let filledCount = 0;
  allInput.forEach((input, index) => {
    const isLast = index === allInput.length - 1;
    const size = isLast ? rest.length : getMaximumLength(input) || 4;
    const part = rest.slice(0, size);
    rest = rest.slice(size);
    if (!part) return;
    setInputValue(input, part);
    filledCount += 1;
  });
  return filledCount;
}

// fillCardIntoScope fills the empty fields of a payment form, and lists whose option matches. It returns how many
// fields it filled.
function fillCardIntoScope(scope: CardScope, anchor: CardField | undefined, card: CardValues): number {
  let filledCount = fillNumber(
    scope.allField.filter((field) => field.kind === "number"),
    card.number,
  );
  for (const field of scope.allField) {
    if (field.kind === "number") continue;
    const element = field.element;
    if (element.tagName === "SELECT") {
      const select = element as HTMLSelectElement;
      if (field.kind === "expiry-month" && card.expiryMonth && chooseOption(select, Number(card.expiryMonth), true)) filledCount += 1;
      if (field.kind === "expiry-year" && card.expiryYear && chooseOption(select, Number(card.expiryYear), false)) filledCount += 1;
      continue;
    }
    const input = element as HTMLInputElement;
    if (input.value.trim() !== "") continue;
    let value = "";
    switch (field.kind) {
      case "name":
        value = card.cardholder;
        break;
      case "security-code":
        value = card.securityCode;
        break;
      case "expiry":
        value = card.expiryMonth && card.expiryYear ? formatExpiry(input, card.expiryMonth, card.expiryYear) : "";
        break;
      case "expiry-month":
        value = card.expiryMonth;
        break;
      case "expiry-year":
        value = card.expiryYear && getMaximumLength(input) === 2 ? card.expiryYear.slice(2) : card.expiryYear;
        break;
    }
    if (!value) continue;
    setInputValue(input, value);
    filledCount += 1;
  }
  anchor?.element.focus();
  return filledCount;
}

// fillCardIntoScopeOf fills the payment form of the field the inline menu is anchored to.
export function fillCardIntoScopeOf(document: Document, element: Element, card: CardValues): number {
  const scope = findCardScopeOf(document, element);
  if (!scope) return 0;
  return fillCardIntoScope(
    scope,
    scope.allField.find((field) => field.element === element),
    card,
  );
}

// fillCardIntoPage fills the payment form with the focused field, or else the page's first one.
export function fillCardIntoPage(document: Document, card: CardValues): number {
  const active = getDeepActiveElement(document);
  const allScope = findAllCardScope(document);
  const focused = active ? allScope.find((scope) => scope.allField.some((field) => field.element === active)) : undefined;
  const scope = focused ?? allScope[0];
  if (!scope) return 0;
  return fillCardIntoScope(
    scope,
    scope.allField.find((field) => field.element === active),
    card,
  );
}

export function hasCardForm(document: Document): boolean {
  return findAllCardScope(document).length > 0;
}

export function isFocusedCardField(document: Document): boolean {
  const active = getDeepActiveElement(document);
  return active !== null && findCardScopeOf(document, active) !== undefined;
}
