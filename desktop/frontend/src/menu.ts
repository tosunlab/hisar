import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

export type MenuCloseReason = "escape" | "tab" | "outside" | "action";

export function useMenu(menu: Ref<HTMLElement | null>, close: (reason: MenuCloseReason) => void, trigger: () => HTMLElement | null = () => null): void {
  const buttons = () => [...(menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') ?? [])];
  watch(menu, (element) => element && buttons()[0]?.focus(), { flush: "post" });
  function handleKey(event: KeyboardEvent): void {
    if (!menu.value) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close("escape");
    } else if (event.key === "Tab") {
      trigger()?.focus();
      close("tab");
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const enabled = buttons();
      const index = enabled.indexOf(document.activeElement as HTMLButtonElement);
      const next = index < 0 ? (event.key === "ArrowDown" ? 0 : enabled.length - 1) : (index + (event.key === "ArrowDown" ? 1 : -1) + enabled.length) % enabled.length;
      enabled[next]?.focus();
    }
  }
  function outside(event: PointerEvent): void {
    if (menu.value && !menu.value.contains(event.target as Node) && !trigger()?.contains(event.target as Node)) close("outside");
  }
  onMounted(() => {
    buttons()[0]?.focus();
    window.addEventListener("keydown", handleKey);
    window.addEventListener("pointerdown", outside);
  });
  onBeforeUnmount(() => {
    window.removeEventListener("keydown", handleKey);
    window.removeEventListener("pointerdown", outside);
  });
}
