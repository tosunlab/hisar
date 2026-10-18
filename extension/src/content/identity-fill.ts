import type { IdentityValues } from "../shared/protocol";
import { fillAddressNear } from "./address-fill";
import { getDeepActiveElement } from "./dom";
import { findAllIdentityScope, findIdentityScopeOf, type IdentityField, type IdentityScope } from "./identity-detection";

// setInputValue sets a value through the prototype setter, so frameworks that track the value see the change.
function setInputValue(input: HTMLInputElement, value: string): void {
  input.focus();
  const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (valueSetter) valueSetter.call(input, value);
  else input.value = value;
  input.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertReplacementText", data: value }));
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

function getValue(field: IdentityField, identity: IdentityValues): string {
  switch (field.kind) {
    case "name":
      return [identity.firstName, identity.middleName, identity.lastName].filter(Boolean).join(" ");
    case "given-name":
      return identity.firstName;
    case "additional-name":
      return identity.middleName;
    case "family-name":
      return identity.lastName;
    case "email":
      return identity.email;
    case "tel":
      return identity.phone;
    case "bday":
      // A date input takes only YYYY-MM-DD.
      return field.element.type === "date" && !/^\d{4}-\d{2}-\d{2}$/.test(identity.birthDate) ? "" : identity.birthDate;
  }
}

// fillIdentityIntoScope fills the scope's empty personal fields, and then the address form beside them with the
// identity's address. It returns how many fields it filled.
function fillIdentityIntoScope(document: Document, scope: IdentityScope, anchor: IdentityField | undefined, identity: IdentityValues): number {
  let filledCount = 0;
  for (const field of scope.allField) {
    const value = getValue(field, identity);
    if (!value || field.element.value.trim() !== "") continue;
    setInputValue(field.element, value);
    filledCount += 1;
  }
  if (identity.address) filledCount += fillAddressNear(document, scope.root, identity.address);
  anchor?.element.focus();
  return filledCount;
}

// fillIdentityIntoScopeOf fills the form of the field the inline menu is anchored to.
export function fillIdentityIntoScopeOf(document: Document, element: Element, identity: IdentityValues): number {
  const scope = findIdentityScopeOf(document, element);
  if (!scope) return 0;
  return fillIdentityIntoScope(
    document,
    scope,
    scope.allField.find((field) => field.element === element),
    identity,
  );
}

// fillIdentityIntoPage fills the form with the focused field, or else the page's first one.
export function fillIdentityIntoPage(document: Document, identity: IdentityValues): number {
  const active = getDeepActiveElement(document);
  const allScope = findAllIdentityScope(document);
  const focused = active ? allScope.find((scope) => scope.allField.some((field) => field.element === active)) : undefined;
  const scope = focused ?? allScope[0];
  if (!scope) return 0;
  return fillIdentityIntoScope(
    document,
    scope,
    scope.allField.find((field) => field.element === active),
    identity,
  );
}

export function hasIdentityForm(document: Document): boolean {
  return findAllIdentityScope(document).length > 0;
}

export function isFocusedIdentityField(document: Document): boolean {
  const active = getDeepActiveElement(document);
  return active !== null && findIdentityScopeOf(document, active) !== undefined;
}
