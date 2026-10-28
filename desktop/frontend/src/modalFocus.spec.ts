import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import AccountDialog from "./components/AccountDialog.vue";
import { useModalFocus } from "./modalFocus";

function tab(shiftKey = false): void {
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true }));
}

describe("modal focus", () => {
  it("captures the current opener when a permanently mounted modal becomes visible", async () => {
    const shown = ref(false);
    const wrapper = mount(defineComponent({
      setup() {
        const panel = ref<HTMLElement | null>(null);
        useModalFocus(panel, () => { shown.value = false; });
        return () => h("div", [
          h("button", { id: "pair-browser" }, "Pair browser"),
          shown.value ? h("div", { ref: panel, role: "dialog", tabindex: -1 }, [h("button", {}, "Cancel")]) : null,
        ]);
      },
    }), { attachTo: document.body });
    await nextTick();
    const opener = wrapper.find("#pair-browser").element as HTMLElement;
    opener.focus();
    shown.value = true;
    await nextTick();
    expect(document.activeElement?.textContent).toBe("Cancel");
    shown.value = false;
    await nextTick();
    expect(document.activeElement).toBe(opener);
    wrapper.unmount();
  });

  it("focuses fields, traps both Tab directions without inert support and restores the opener", async () => {
    const opener = document.createElement("button");
    opener.textContent = "Open";
    opener.setAttribute("aria-hidden", "false");
    document.body.append(opener);
    opener.focus();
    const wrapper = mount(AccountDialog, {
      attachTo: document.body,
      props: { title: "Change name", subtitle: "Choose a name.", icon: "edit" },
      slots: { default: '<input aria-label="Name"><button class="ghost">Cancel</button><button>Save</button>' },
    });
    await nextTick();
    const all = wrapper.findAll("input, button").map((control) => control.element);
    expect(document.activeElement).toBe(all[0]);
    expect(opener.getAttribute("aria-hidden")).toBe("true");
    expect(opener.hasAttribute("inert")).toBe(true);
    tab(true);
    expect(document.activeElement).toBe(all[2]);
    tab();
    expect(document.activeElement).toBe(all[0]);
    opener.focus();
    expect(document.activeElement).toBe(all[0]);
    const dialog = wrapper.find('[role="dialog"]');
    expect(document.getElementById(dialog.attributes("aria-labelledby")!)?.textContent).toBe("Change name");
    expect(document.getElementById(dialog.attributes("aria-describedby")!)?.textContent).toBe("Choose a name.");
    wrapper.unmount();
    expect(document.activeElement).toBe(opener);
    expect(opener.hasAttribute("inert")).toBe(false);
    expect(opener.getAttribute("aria-hidden")).toBe("false");
    opener.remove();
  });

  it("starts destructive confirmations on Cancel and lets only the top dialog handle Escape", async () => {
    const nested = ref(false);
    const closed = ref(false);
    const wrapper = mount(defineComponent({
      setup: () => () => h(AccountDialog, { title: "Sharing", subtitle: "People", icon: "users", onClose: () => { closed.value = true; } }, {
        default: () => [h("button", { id: "review", onClick: () => { nested.value = true; } }, "Review recipient"), nested.value ? h(AccountDialog, { title: "Delete?", subtitle: "Cannot undo", icon: "trash", isDanger: true, onClose: () => { nested.value = false; } }, { default: () => [h("input", { "aria-label": "Confirmation" }), h("button", { class: "ghost" }, "Cancel"), h("button", {}, "Delete")] }) : null],
      }),
    }), { attachTo: document.body });
    await nextTick();
    const review = wrapper.find("#review");
    (review.element as HTMLElement).focus();
    await review.trigger("click");
    await nextTick();
    expect(document.activeElement?.textContent).toBe("Cancel");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await nextTick();
    expect(closed.value).toBe(false);
    expect(nested.value).toBe(false);
    expect(document.activeElement).toBe(review.element);
    wrapper.unmount();
  });
});
