import { afterEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import ReadIssuesPanel from "./ReadIssuesPanel.vue";
import { api } from "../api";
import { store } from "../store";

vi.mock("../api", () => ({ api: { repairVaultName: vi.fn().mockResolvedValue(undefined) }, getErrorMessage: String }));
vi.mock("../store", () => ({ store: { allVault: [] }, retryReads: vi.fn() }));
enableAutoUnmount(afterEach);

describe("Vault name recovery drafts", () => {
  it("keeps names and enablement tied to the reviewed vault and revision", async () => {
    const { reactive } = await import("vue");
    const vaults = reactive(["personal", "team"].map((id) => ({ id, name: id, type: "shared", role: "owner", itemCount: 0,
      readIssues: [{ vaultId: id, kind: "vault-name", revision: "r1", canRename: true }],
    })));
    store.allVault = vaults as typeof store.allVault;
    const wrapper = mount(ReadIssuesPanel);
    const inputs = wrapper.findAll("input");
    expect(inputs.map((input) => input.attributes("aria-label"))).toEqual(["New name for personal", "New name for team"]);
    await inputs[0].setValue("Recovered Personal");
    expect((inputs[1].element as HTMLInputElement).value).toBe("");
    const repairs = () => wrapper.findAll("button").filter((button) => button.text() === "Repair name");
    expect(repairs().map((button) => button.attributes("disabled") !== undefined)).toEqual([false, true]);
    await inputs[1].setValue("Recovered Team");
    await repairs()[1].trigger("click");
    await flushPromises();
    expect(api.repairVaultName).toHaveBeenCalledWith("team", "r1", "Recovered Team");
    expect((inputs[0].element as HTMLInputElement).value).toBe("Recovered Personal");
    vaults[0].readIssues[0].revision = "r2";
    await flushPromises();
    expect((wrapper.findAll("input")[0].element as HTMLInputElement).value).toBe("");
    expect(repairs()[0].attributes("disabled")).toBeDefined();
  });
});
