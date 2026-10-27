import { beforeEach, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { allTestVault, createItem, createStatus } from "../testing";
import { store } from "../store";
import ItemEditor from "./ItemEditor.vue";

vi.mock("../api", async () => {
  // The hoisted API mock loads its fixture before the store imports the API.
  const { createApiMock, createItem } = await import("../testing");
  return { api: createApiMock(), createEmptyItem: createItem, getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));


beforeEach(() => {
  Object.assign(store, {
    status: createStatus("unlocked"), allVault: allTestVault, selectedVaultID: "personal", editorMode: "edit",
    editorDraft: null, discardDraftPrompt: null, isBusy: false,
    selectedItem: createItem({ fields: [
      { name: "Recovery code", value: "123", concealed: true },
      { name: "API TOKEN", value: "456", concealed: false },
      { name: "", value: "", concealed: false },
    ] }),
  });
});

it("names each custom field group and its individual controls, including empty names and renames", async () => {
  const wrapper = mount(ItemEditor);
  const rows = wrapper.findAll('.custom-field[role="group"]');
  for (const [index, name] of ["Recovery code", "API TOKEN", "Field 3"].entries()) {
    expect(rows[index].attributes("aria-label")).toBe(name);
    expect(rows[index].get(`input[name="fieldValue${index}"]`).attributes("aria-label")).toBe(`Value for ${name}`);
    expect(rows[index].get('input[type="checkbox"]').attributes("aria-label")).toBe(`Keep ${name} secret`);
    expect(rows[index].findAll("button").at(-1)?.attributes("aria-label")).toBe(`Remove ${name}`);
  }
  await rows[0].get('input[name="fieldName0"]').setValue("Backup SSH token");
  expect(rows[0].attributes("aria-label")).toBe("Backup SSH token");
  expect(rows[0].get('input[name="fieldValue0"]').attributes("aria-label")).toBe("Value for Backup SSH token");
  expect(rows[0].get('input[type="checkbox"]').attributes("aria-label")).toBe("Keep Backup SSH token secret");
  expect(rows[0].findAll("button").at(-1)?.attributes("aria-label")).toBe("Remove Backup SSH token");
  await rows[1].get('input[type="checkbox"]').setValue(true);
  expect(rows[1].get('input[name="fieldValue1"]').attributes("type")).toBe("password");
  await rows[1].findAll("button").at(-1)?.trigger("click");
  expect(wrapper.findAll('.custom-field[role="group"]')).toHaveLength(2);
  wrapper.unmount();
});
