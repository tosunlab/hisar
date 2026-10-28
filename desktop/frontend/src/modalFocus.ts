import { onBeforeUnmount, watch, type Ref } from "vue";

type Modal = { panel: HTMLElement; opener: HTMLElement | null; close: () => void };
const stack: Modal[] = [];
const hidden = new Map<HTMLElement, { inert: string | null; ariaHidden: string | null }>();
const focusableSelector = 'button:not(:disabled), a[href], input:not(:disabled):not([type="hidden"]), textarea:not(:disabled), select:not(:disabled), [tabindex]:not([tabindex="-1"])';

function controls(panel: HTMLElement): HTMLElement[] {
  return Array.from(panel.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => {
    for (let parent: HTMLElement | null = element; parent && parent !== panel; parent = parent.parentElement) {
      const style = getComputedStyle(parent);
      if (parent.hidden || style.display === "none" || style.visibility === "hidden") return false;
    }
    return element.tabIndex >= 0;
  });
}

function isolateTopModal(): void {
  for (const [element, previous] of hidden) {
    if (previous.inert === null) element.removeAttribute("inert");
    else element.setAttribute("inert", previous.inert);
    if (previous.ariaHidden === null) element.removeAttribute("aria-hidden");
    else element.setAttribute("aria-hidden", previous.ariaHidden);
  }
  hidden.clear();
  let child: HTMLElement | undefined = stack[stack.length - 1]?.panel;
  while (child?.parentElement) {
    for (const sibling of child.parentElement.children) {
      if (sibling === child || !(sibling instanceof HTMLElement)) continue;
      hidden.set(sibling, { inert: sibling.getAttribute("inert"), ariaHidden: sibling.getAttribute("aria-hidden") });
      sibling.setAttribute("inert", "");
      sibling.setAttribute("aria-hidden", "true");
    }
    child = child.parentElement;
    if (child === document.body) break;
  }
}

function focusInitial(modal: Modal): void {
  const all = controls(modal.panel);
  const initial = all.find((element) => element.hasAttribute("data-dialog-initial-focus"))
    ?? (modal.panel.hasAttribute("data-dialog-safe-action") ? all.find((element) => element.matches("button.ghost")) : undefined)
    ?? all.find((element) => element.hasAttribute("autofocus"))
    ?? all.find((element) => element.matches('input:not([type="checkbox"]):not([type="radio"]), textarea, select'))
    ?? all.find((element) => element.matches("button.ghost"))
    ?? all[0]
    ?? modal.panel;
  initial.focus();
}

function handleKey(event: KeyboardEvent): void {
  const modal = stack[stack.length - 1];
  if (!modal) return;
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopImmediatePropagation();
    modal.close();
  } else if (event.key === "Tab") {
    event.preventDefault();
    event.stopImmediatePropagation();
    const all = controls(modal.panel);
    const index = all.indexOf(document.activeElement as HTMLElement);
    if (!all.length) modal.panel.focus();
    else all[(index + (event.shiftKey ? -1 : 1) + all.length) % all.length]!.focus();
  }
}

function containFocus(event: FocusEvent): void {
  const modal = stack[stack.length - 1];
  if (modal && !modal.panel.contains(event.target as Node)) focusInitial(modal);
}

function blockBackground(event: Event): void {
  const modal = stack[stack.length - 1];
  if (modal && !modal.panel.parentElement?.contains(event.target as Node)) {
    event.preventDefault();
    event.stopImmediatePropagation();
  }
}

export function useModalFocus(panel: Ref<HTMLElement | null>, close: () => void): void {
  let modal: Modal | null = null;
  function remove(): void {
    if (!modal) return;
    const wasTop = stack[stack.length - 1] === modal;
    stack.splice(stack.indexOf(modal), 1);
    const restore = modal.opener;
    modal = null;
    isolateTopModal();
    if (!stack.length) {
      window.removeEventListener("keydown", handleKey, true);
      document.removeEventListener("focusin", containFocus, true);
      document.removeEventListener("pointerdown", blockBackground, true);
      document.removeEventListener("click", blockBackground, true);
    }
    if (wasTop) {
      if (restore?.isConnected && (!stack.length || stack[stack.length - 1]!.panel.contains(restore))) restore.focus();
      else if (stack.length) focusInitial(stack[stack.length - 1]!);
    }
  }
  watch(panel, (element) => {
    const opener = document.activeElement instanceof HTMLElement && document.activeElement !== document.body ? document.activeElement : null;
    remove();
    if (!element) return;
    modal = { panel: element, opener, close };
    stack.push(modal);
    window.addEventListener("keydown", handleKey, true);
    document.addEventListener("focusin", containFocus, true);
    document.addEventListener("pointerdown", blockBackground, true);
    document.addEventListener("click", blockBackground, true);
    focusInitial(modal);
    isolateTopModal();
  }, { flush: "post" });
  onBeforeUnmount(remove);
}
