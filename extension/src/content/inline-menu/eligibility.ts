import type { InlineMenuPurpose } from "../../shared/messages";
import { getAutocompleteTokens } from "../detection";
import { isInputElement, isVisible } from "../dom";
import { findScopeOfField } from "../fill";
import { isFieldLargeEnough, type Rect } from "./placement";

export const allOwnHostTagName = ["HISAR-SAVE-BAR", "HISAR-INLINE-MENU"];

export function isFrameEligibleForInlineMenu(window: Window): boolean {
  const { protocol, origin } = window.location;
  if ((protocol !== "https:" && protocol !== "http:") || origin === "null") return false;
  return Array.from(window.location.ancestorOrigins ?? []).every((ancestorOrigin) => ancestorOrigin === origin);
}

function isInsideOwnHost(field: Element): boolean {
  const root = field.getRootNode();
  return root instanceof ShadowRoot && allOwnHostTagName.includes(root.host.tagName);
}

export function findInlineMenuPurpose(document: Document, element: Element, getRect: (element: Element) => Rect): InlineMenuPurpose | undefined {
  if (!isInputElement(element) || element.disabled || element.readOnly || isInsideOwnHost(element)) return undefined;
  if (!isVisible(element)) return undefined;
  const hasLayout = document.documentElement.getClientRects().length > 0;
  if (hasLayout && !isFieldLargeEnough(getRect(element))) return undefined;
  const scope = findScopeOfField(document, element);
  if (!scope) return undefined;
  if (scope.kind === "login" || scope.kind === "username-only") return "login";
  const isPassword = scope.allPasswordField.includes(element);
  if (isPassword && !getAutocompleteTokens(element).includes("current-password")) {
    const isCurrentPasswordOfChangeForm = scope.kind === "change-password" && scope.allPasswordField.length >= 3 && scope.allPasswordField[0] === element;
    return isCurrentPasswordOfChangeForm ? undefined : "new-password";
  }
  return undefined;
}
