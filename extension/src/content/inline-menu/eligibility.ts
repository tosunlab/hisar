import { readAncestorOrigins } from "../../shared/frame-ancestry";
import type { InlineMenuPurpose } from "../../shared/messages";
import { findAddressScopeOf } from "../address-detection";
import { findCardScopeOf } from "../card-detection";
import { findIdentityScopeOf } from "../identity-detection";
import { getAutocompleteTokens } from "../detection";
import { isInputElement, isVisible } from "../dom";
import { findScopeOfField } from "../fill";
import { findOneTimeCodeTarget } from "../one-time-code";
import { computeUnionRect, isFieldLargeEnough, type Rect } from "./placement";

export const allOwnHostTagName = ["HISAR-SAVE-BAR", "HISAR-INLINE-MENU", "HISAR-NOTICE"];

export function isFrameEligibleForInlineMenu(window: Window): boolean {
  const { protocol, origin } = window.location;
  if ((protocol !== "https:" && protocol !== "http:") || origin === "null") return false;
  return readAncestorOrigins(window).every((ancestorOrigin) => ancestorOrigin === origin);
}

function isInsideOwnHost(field: Element): boolean {
  const root = field.getRootNode();
  return root instanceof ShadowRoot && allOwnHostTagName.includes(root.host.tagName);
}

export function findInlineMenuPurpose(document: Document, element: Element, getRect: (element: Element) => Rect): InlineMenuPurpose | undefined {
  if (!isInputElement(element) || element.disabled || element.readOnly || isInsideOwnHost(element)) return undefined;
  if (!isVisible(element)) return undefined;
  const hasLayout = document.documentElement.getClientRects().length > 0;
  const allCodeField = findOneTimeCodeTarget(element);
  if (allCodeField) {
    if (hasLayout && !isFieldLargeEnough(computeUnionRect(allCodeField.map(getRect)))) return undefined;
    return "one-time-code";
  }
  if (hasLayout && !isFieldLargeEnough(getRect(element))) return undefined;
  // A payment form is told apart first: its security code may be a password field, which would make it look like a login.
  if (findCardScopeOf(document, element)) return "card";
  const scope = findScopeOfField(document, element);
  // A field that is not part of a login takes an address when it belongs to an address form, else an identity when it is
  // a name, email or phone field of a form with personal details.
  if (!scope) {
    if (findAddressScopeOf(document, element)) return "address";
    return findIdentityScopeOf(document, element) ? "identity" : undefined;
  }
  if (scope.kind === "login" || scope.kind === "username-only") return "login";
  const isPassword = scope.allPasswordField.includes(element);
  // A sign-up form's name and email fields take an identity; its password fields a suggested password.
  if (!isPassword && scope.kind === "sign-up") return findIdentityScopeOf(document, element) ? "identity" : undefined;
  if (isPassword && !getAutocompleteTokens(element).includes("current-password")) {
    const isCurrentPasswordOfChangeForm = scope.kind === "change-password" && scope.allPasswordField.length >= 3 && scope.allPasswordField[0] === element;
    return isCurrentPasswordOfChangeForm ? undefined : "new-password";
  }
  return undefined;
}
