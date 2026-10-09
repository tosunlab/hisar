import type { OffscreenCopyMessage } from "../shared/messages";

export function copyText(document: Document, text: string): boolean {
  const textArea = document.createElement("textarea");
  textArea.value = text;
  document.body.append(textArea);
  textArea.select();
  try {
    return document.execCommand("copy");
  } finally {
    textArea.remove();
  }
}

export function isOffscreenCopyMessage(message: unknown, sender: { id?: string; tab?: unknown }, extensionID: string): message is OffscreenCopyMessage {
  const candidate = message as Partial<OffscreenCopyMessage> | undefined;
  return (
    candidate?.target === "offscreen" &&
    candidate.type === "offscreen.copy" &&
    typeof candidate.text === "string" &&
    sender.id === extensionID &&
    sender.tab === undefined
  );
}
