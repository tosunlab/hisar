import { Clipboard, closeMainWindow, showHUD } from "@raycast/api";
import type { CopyableField, Hisar, ItemReference } from "../hisar";
import { showHisarError } from "./feedback";

const labelByField: Record<CopyableField, string> = {
  username: "Username",
  password: "Password",
  "one-time code": "One-time code",
};

/** copyField has Hisar copy the field, so the value never enters Raycast; Hisar clears it later. */
export async function copyField(hisar: Hisar, item: ItemReference, field: CopyableField): Promise<void> {
  try {
    const { clearAfterSeconds } = await hisar.copy(item, field);
    await showHUD(`${labelByField[field]} copied, clears in ${clearAfterSeconds} seconds`);
  } catch (error) {
    await showHisarError(error);
  }
}

/** pasteValue reads one value from Hisar and pastes it into the app in front of Raycast, keeping it nowhere. */
export async function pasteValue(read: () => Promise<string>): Promise<void> {
  try {
    const value = await read();
    await closeMainWindow();
    await Clipboard.paste(value);
  } catch (error) {
    await showHisarError(error);
  }
}

/** copyGeneratedPassword copies a new password in Raycast, concealed so clipboard history skips it. */
export async function copyGeneratedPassword(generate: () => Promise<string>): Promise<void> {
  try {
    await Clipboard.copy(await generate(), { concealed: true });
    await showHUD("Password copied");
  } catch (error) {
    await showHisarError(error);
  }
}
