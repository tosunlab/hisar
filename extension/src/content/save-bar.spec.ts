import { afterEach, describe, expect, it, vi } from "vitest";
import type { SaveOfferView } from "../shared/messages";
import { describeSaveOffer, showSaveBar } from "./save-bar";

const newOffer: SaveOfferView = { offerID: "offer-1", kind: "save", host: "example.com", username: "alex" };
const changedOffer: SaveOfferView = { offerID: "offer-2", kind: "update", host: "example.com", username: "alex", title: "Example" };

function findButton(shadowRoot: ShadowRoot, action: string): HTMLButtonElement {
  return shadowRoot.querySelector<HTMLButtonElement>(`.actions button[data-action="${action}"]`)!;
}

afterEach(() => {
  document.documentElement.querySelectorAll("hisar-save-bar").forEach((element) => element.remove());
  vi.useRealTimers();
});

describe("describeSaveOffer", () => {
  it("asks to save a new login and to update a changed one", () => {
    expect(describeSaveOffer(newOffer)).toEqual({ title: "Save this login for example.com in Hisar?", primaryLabel: "Save" });
    expect(describeSaveOffer(changedOffer)).toEqual({ title: "Update the password for Example in Hisar?", primaryLabel: "Update" });
  });
});

describe("showSaveBar", () => {
  it("renders inside a closed shadow root that the page cannot reach", () => {
    const bar = showSaveBar(document, newOffer, async () => ({ ok: true }));
    expect(bar.host.shadowRoot).toBeNull();
    expect(bar.shadowRoot.textContent).toContain("Save this login for example.com in Hisar?");
    expect(bar.shadowRoot.textContent).toContain("alex");
    expect(document.querySelectorAll("hisar-save-bar")).toHaveLength(1);
  });

  it("does not render the username as HTML", () => {
    const bar = showSaveBar(document, { ...newOffer, username: "<img src=x onerror=alert(1)>" }, async () => ({ ok: true }));
    expect(bar.shadowRoot.querySelector("img")).toBeNull();
  });

  it("replaces an existing bar", () => {
    showSaveBar(document, newOffer, async () => ({ ok: true }));
    showSaveBar(document, changedOffer, async () => ({ ok: true }));
    expect(document.querySelectorAll("hisar-save-bar")).toHaveLength(1);
  });

  it("shows Update for a changed login and reports the save action", async () => {
    vi.useFakeTimers();
    const onAction = vi.fn(async () => ({ ok: true }));
    const bar = showSaveBar(document, changedOffer, onAction);
    const primaryButton = findButton(bar.shadowRoot, "save");
    expect(primaryButton.textContent).toBe("Update");
    primaryButton.click();
    await vi.waitFor(() => expect(bar.shadowRoot.textContent).toContain("Password updated in Hisar."));
    expect(onAction).toHaveBeenCalledWith("save");
    vi.advanceTimersByTime(3000);
    expect(bar.host.isConnected).toBe(false);
  });

  it("keeps the bar open with the error when saving fails", async () => {
    const bar = showSaveBar(document, newOffer, async () => ({ ok: false, message: "Hisar is locked" }));
    findButton(bar.shadowRoot, "save").click();
    await vi.waitFor(() => expect(bar.shadowRoot.querySelector(".message.error")?.textContent).toBe("Hisar is locked"));
    expect(bar.host.isConnected).toBe(true);
    expect(findButton(bar.shadowRoot, "save").disabled).toBe(false);
  });

  it.each(["not-now", "never"])("closes after %s", async (action) => {
    const onAction = vi.fn(async () => ({ ok: true }));
    const bar = showSaveBar(document, newOffer, onAction);
    findButton(bar.shadowRoot, action).click();
    await vi.waitFor(() => expect(bar.host.isConnected).toBe(false));
    expect(onAction).toHaveBeenCalledWith(action);
  });

  it("lets the user change the title, username and vault of a new login before saving", async () => {
    const onAction = vi.fn(async () => ({ ok: true }));
    const offer = {
      ...newOffer,
      defaultTitle: "example.com",
      allVault: [
        { vaultID: "vault-personal", name: "Personal" },
        { vaultID: "vault-family", name: "Family" },
      ],
    };
    const bar = showSaveBar(document, offer, onAction);
    const title = bar.shadowRoot.querySelector<HTMLInputElement>('[data-field="title"]')!;
    const username = bar.shadowRoot.querySelector<HTMLInputElement>('[data-field="username"]')!;
    const vault = bar.shadowRoot.querySelector<HTMLSelectElement>('[data-field="vault"]')!;
    expect([title.value, username.value, vault.value]).toEqual(["example.com", newOffer.username, "vault-personal"]);
    expect(Array.from(vault.options).map((option) => option.text)).toEqual(["Personal", "Family"]);
    title.value = "Example shop";
    username.value = "alex";
    vault.value = "vault-family";
    findButton(bar.shadowRoot, "save").click();
    await vi.waitFor(() => expect(onAction).toHaveBeenCalledWith("save", { title: "Example shop", username: "alex", vaultID: "vault-family" }));
  });

  it("keeps the page's shortcuts from acting on what is typed in the bar", () => {
    const offer = { ...newOffer, defaultTitle: "example.com", allVault: [{ vaultID: "vault-personal", name: "Personal" }] };
    const bar = showSaveBar(document, offer, async () => ({ ok: true }));
    const pageListener = vi.fn();
    document.addEventListener("keydown", pageListener);
    bar.shadowRoot.querySelector('[data-field="title"]')!.dispatchEvent(new KeyboardEvent("keydown", { key: "s", bubbles: true, composed: true }));
    expect(pageListener).not.toHaveBeenCalled();
    expect(bar.shadowRoot.querySelector('[data-field="vault"]')).toBeNull();
    document.removeEventListener("keydown", pageListener);
  });

  it("closes with the close button as Not now", async () => {
    const onAction = vi.fn(async () => ({ ok: true }));
    const bar = showSaveBar(document, newOffer, onAction);
    bar.shadowRoot.querySelector<HTMLButtonElement>(".close")!.click();
    await vi.waitFor(() => expect(bar.host.isConnected).toBe(false));
    expect(onAction).toHaveBeenCalledWith("not-now");
  });
});
