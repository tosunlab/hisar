import { Clipboard, closeMainWindow, showHUD, showToast, Toast } from "@raycast/api";
import type { CopyableField, Hisar, ItemReference, ShareLinkOptions } from "../hisar";
import { showHisarError } from "./feedback";
import { isClosingWindowAfterCopy } from "./preferences";

const labelByField: Record<CopyableField, string> = {
  username: "Username",
  password: "Password",
  "one-time code": "One-time code",
};

/** announceCopy closes Raycast with a HUD, or keeps it open with a toast when the preference says so. */
async function announceCopy(message: string): Promise<void> {
  if (isClosingWindowAfterCopy()) await showHUD(message);
  else await showToast({ style: Toast.Style.Success, title: message });
}

/** copyField has Hisar copy the field, so the value never enters Raycast; Hisar clears it later. */
export async function copyField(hisar: Hisar, item: ItemReference, field: CopyableField): Promise<void> {
  try {
    const { clearAfterSeconds } = await hisar.copy(item, field);
    await announceCopy(`${labelByField[field]} copied, clears in ${clearAfterSeconds} seconds`);
  } catch (error) {
    await showHisarError(error);
  }
}

/** pasteValue reads one value and pastes it into the app in front of Raycast, keeping it nowhere. */
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
    await announceCopy("Password copied");
  } catch (error) {
    await showHisarError(error);
  }
}

/** openItem shows the item, or its editor, in Hisar's window, and gets Raycast out of its way. */
export async function openItem(hisar: Hisar, item: ItemReference, isEditing: boolean): Promise<void> {
  try {
    await hisar.open(item, { isEditing });
    await closeMainWindow();
  } catch (error) {
    await showHisarError(error);
  }
}

/** shareItem has Hisar create a share link, which Hisar asks the user to confirm, then copies its URL concealed. */
export async function shareItem(hisar: Hisar, item: ItemReference, options: ShareLinkOptions): Promise<boolean> {
  const toast = await showToast({
    style: Toast.Style.Animated,
    title: "Creating share link",
    message: "Confirm it in Hisar",
  });
  try {
    await Clipboard.copy(await hisar.share(item, options), { concealed: true });
    await toast.hide();
    await announceCopy("Share link copied");
    return true;
  } catch (error) {
    await showHisarError(error);
    return false;
  }
}
