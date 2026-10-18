import { collectAllDeepElement, containsAcrossShadow, getParentAcrossShadow, isOffPage, isTransparent, isVisible } from "./dom";

// IdentityKind is what a personal field holds: the whole name, or one of its parts.
export type IdentityKind = "name" | "given-name" | "additional-name" | "family-name" | "email" | "tel" | "bday";

export interface IdentityField {
  element: HTMLInputElement;
  kind: IdentityKind;
  isByAutocomplete: boolean;
}

export interface IdentityScope {
  root: Element;
  allField: IdentityField[];
}

const allAutocompleteKind: Record<string, IdentityKind> = {
  name: "name",
  "given-name": "given-name",
  "additional-name": "additional-name",
  "family-name": "family-name",
  email: "email",
  tel: "tel",
  "tel-national": "tel",
  bday: "bday",
};

const allIdentityInputType = new Set(["", "text", "email", "tel", "date"]);

// The hint patterns, tried in this order: a name's parts before the whole name, whose hint ("name") they share.
const allHintKind: [IdentityKind, RegExp][] = [
  ["given-name", /first[\s_-]*name|given[\s_-]*name|\bfname\b|forename|vorname|pr[ée]nom|\bnombre\b(?![\s_-]*completo)|\bisim\b|\bad[ıi]n[ıi]z\b/i],
  ["additional-name", /middle[\s_-]*(name|initial)|\bmname\b/i],
  ["family-name", /last[\s_-]*name|family[\s_-]*name|surname|\blname\b|nachname|\bnom\b|apellido|soyad/i],
  ["name", /full[\s_-]*name|your[\s_-]*name|\bname\b|nombre[\s_-]*completo|ad[\s_-]*soyad|\bisim[\s_-]*soyisim/i],
  ["email", /e-?mail|correo|courriel|e-?posta/i],
  ["tel", /phone|mobile|\btel\b|telefon|tel[eé]fono|\bgsm\b|\bcell\b/i],
  ["bday", /birth|\bdob\b|bday|geburt|naissance|nacimiento|do[gğ]um[\s_-]*tarih/i],
];

// Fields that hold something else with a name, such as a username, a company or a card's holder.
const excludedHintPattern = /user|login|search|coupon|promo|company|organi[sz]ation|business|card|holder|captcha|password|nick|display|screen|pet|file|domain|url|website/i;

function getAutocompleteTokens(element: HTMLInputElement): string[] {
  return (element.getAttribute("autocomplete") ?? "").toLowerCase().split(/\s+/).filter(Boolean);
}

function getLabelText(element: HTMLInputElement): string {
  return Array.from(element.labels ?? [])
    .map((label) => label.textContent ?? "")
    .join(" ");
}

function getHints(element: HTMLInputElement): string {
  return [element.name, element.id, element.getAttribute("placeholder"), element.getAttribute("aria-label"), getLabelText(element)]
    .filter((hint): hint is string => Boolean(hint))
    .join(" ");
}

function isIdentityElement(element: Element): element is HTMLInputElement {
  return element.tagName === "INPUT" && allIdentityInputType.has((element.getAttribute("type") ?? "").toLowerCase());
}

// isFillable is a field Hisar may fill: shown, enabled and writable. A fill writes every empty field of the form, so a
// transparent field, or one moved off the page, counts as hidden.
function isFillable(element: HTMLInputElement): boolean {
  if (element.disabled || element.readOnly) return false;
  return isVisible(element) && !isTransparent(element) && !isOffPage(element);
}

// classifyIdentityField says what personal value a field takes, by its autocomplete attribute or else its type, name,
// id, placeholder, label and aria-label, or returns undefined for any other field.
export function classifyIdentityField(element: Element): IdentityField | undefined {
  if (!isIdentityElement(element) || !isFillable(element)) return undefined;
  const allToken = getAutocompleteTokens(element);
  const autocompleteKind = allToken.map((token) => allAutocompleteKind[token]).find((kind) => kind !== undefined);
  if (autocompleteKind) return { element, kind: autocompleteKind, isByAutocomplete: true };
  // Another token names what the field is, such as username or cc-name.
  if (allToken.some((token) => token !== "on" && token !== "off" && !token.startsWith("section-") && token !== "shipping" && token !== "billing")) return undefined;
  const type = element.type.toLowerCase();
  if (type === "email") return { element, kind: "email", isByAutocomplete: false };
  if (type === "tel") return { element, kind: "tel", isByAutocomplete: false };
  const hints = getHints(element);
  if (!hints || excludedHintPattern.test(hints)) return undefined;
  const kind = allHintKind.find(([, pattern]) => pattern.test(hints))?.[0];
  return kind ? { element, kind, isByAutocomplete: false } : undefined;
}

const maximumFormlessDepth = 8;

function findFormlessRoot(field: IdentityField, allFormless: IdentityField[]): Element {
  let current = getParentAcrossShadow(field.element);
  for (let depth = 0; current && depth < maximumFormlessDepth; depth += 1) {
    const ancestor = current;
    if (allFormless.some((other) => other !== field && containsAcrossShadow(ancestor, other.element))) return ancestor;
    if (ancestor.tagName === "BODY") break;
    current = getParentAcrossShadow(ancestor);
  }
  return field.element.ownerDocument.body;
}

// isIdentityScope holds personal details: two kinds of field, such as a name and an email, or one named by its
// autocomplete attribute; a lone "Email" field found by its hints is a newsletter box, not a form to fill.
function isIdentityScope(allField: IdentityField[]): boolean {
  return allField.some((field) => field.isByAutocomplete) || new Set(allField.map((field) => field.kind)).size >= 2;
}

// findAllIdentityScope groups the page's personal fields by their form, or for fields outside any form by the
// container they share.
export function findAllIdentityScope(document: Document): IdentityScope[] {
  const allField = collectAllDeepElement(document, isIdentityElement)
    .map(classifyIdentityField)
    .filter((field): field is IdentityField => field !== undefined);
  const allFormless = allField.filter((field) => !field.element.form);
  const formlessRoots = new Map(allFormless.map((field) => [field, findFormlessRoot(field, allFormless)]));
  const allScope: IdentityScope[] = [];
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
  return allScope.filter((scope) => isIdentityScope(scope.allField));
}

// findIdentityScopeOf returns the form of personal details that holds element, or undefined.
export function findIdentityScopeOf(document: Document, element: Element): IdentityScope | undefined {
  return findAllIdentityScope(document).find((scope) => scope.allField.some((field) => field.element === element));
}
