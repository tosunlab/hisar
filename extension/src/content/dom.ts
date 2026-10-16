export function collectAllDeepElement<T extends Element>(root: ParentNode, predicate: (element: Element) => element is T): T[] {
  const allMatch: T[] = [];
  const visit = (parent: ParentNode): void => {
    for (const child of Array.from(parent.children)) {
      if (predicate(child)) allMatch.push(child);
      if (child.shadowRoot) visit(child.shadowRoot);
      visit(child);
    }
  };
  visit(root);
  return allMatch;
}

export function isInputElement(element: Element): element is HTMLInputElement {
  return element.tagName === "INPUT";
}

export function getParentAcrossShadow(element: Element): Element | null {
  if (element.parentElement) return element.parentElement;
  const root = element.getRootNode();
  return root instanceof ShadowRoot ? root.host : null;
}

export function containsAcrossShadow(ancestor: Element, element: Element): boolean {
  let current: Element | null = element;
  while (current) {
    if (current === ancestor) return true;
    current = getParentAcrossShadow(current);
  }
  return false;
}

export function getDeepActiveElement(document: Document): Element | null {
  let active: Element | null = document.activeElement;
  while (active?.shadowRoot?.activeElement) active = active.shadowRoot.activeElement;
  return active;
}

function hasLayout(document: Document): boolean {
  return document.documentElement.getClientRects().length > 0;
}

// isTransparent reports whether the element or a container of it is drawn fully, or almost fully, transparent.
export function isTransparent(element: Element): boolean {
  const view = element.ownerDocument.defaultView;
  if (!view) return false;
  for (let current: Element | null = element; current; current = getParentAcrossShadow(current)) {
    const opacity = Number.parseFloat(view.getComputedStyle(current).opacity);
    if (!Number.isNaN(opacity) && opacity < 0.05) return true;
  }
  return false;
}

// isOffPage reports whether the element lies wholly above or left of the page, as a field moved to left: -9999px does,
// where no scrolling shows it. Without layout, as in tests, nothing is.
export function isOffPage(element: Element): boolean {
  const view = element.ownerDocument.defaultView;
  if (!view || !hasLayout(element.ownerDocument)) return false;
  const rect = element.getBoundingClientRect();
  return rect.right + view.scrollX <= 0 || rect.bottom + view.scrollY <= 0;
}

export function isVisible(element: Element): boolean {
  if (!element.isConnected) return false;
  const view = element.ownerDocument.defaultView;
  if (!view) return false;
  let current: Element | null = element;
  while (current) {
    if (current.hasAttribute("hidden")) return false;
    const style = view.getComputedStyle(current);
    if (style.display === "none") return false;
    if (current === element && (style.visibility === "hidden" || style.visibility === "collapse")) return false;
    current = getParentAcrossShadow(current);
  }
  if (!hasLayout(element.ownerDocument)) return true;
  const rect = element.getBoundingClientRect();
  return rect.width > 1 && rect.height > 1;
}
