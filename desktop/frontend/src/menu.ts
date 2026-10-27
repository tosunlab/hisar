import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

export type MenuCloseReason = "escape" | "tab" | "outside" | "action" | "destination";

export function useMenu(menu: Ref<HTMLElement | null>, close: (reason: MenuCloseReason) => void, trigger: () => HTMLElement | null = () => null): (reason: MenuCloseReason) => void {
  const buttons = () => [...(menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') ?? [])];
  watch(menu, (element) => element && buttons()[0]?.focus(), { flush: "post" });
  function dismiss(reason: MenuCloseReason): void {
    const invoker = trigger();
    if ((reason === "escape" || reason === "tab" || reason === "action") && invoker?.isConnected) invoker.focus();
    close(reason);
  }
  function handleKey(event: KeyboardEvent): void {
    if (!menu.value) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      dismiss("escape");
    } else if (event.key === "Tab") {
      dismiss("tab");
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const enabled = buttons();
      const index = enabled.indexOf(document.activeElement as HTMLButtonElement);
      const next = index < 0 ? (event.key === "ArrowDown" ? 0 : enabled.length - 1) : (index + (event.key === "ArrowDown" ? 1 : -1) + enabled.length) % enabled.length;
      enabled[next]?.focus();
    }
  }
  function outside(event: PointerEvent): void {
    if (menu.value && !menu.value.contains(event.target as Node) && !trigger()?.contains(event.target as Node)) dismiss("outside");
  }
  onMounted(() => {
    buttons()[0]?.focus();
    window.addEventListener("keydown", handleKey, true);
    window.addEventListener("pointerdown", outside);
  });
  onBeforeUnmount(() => {
    window.removeEventListener("keydown", handleKey, true);
    window.removeEventListener("pointerdown", outside);
  });
  return dismiss;
}
