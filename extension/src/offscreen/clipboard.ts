export interface ClipboardAccess {
  writeText(text: string): void;
  readText(): string | undefined;
}

export class ClipboardClearer {
  private pendingTimer: ReturnType<typeof setTimeout> | undefined;
  private pendingText: string | undefined;

  constructor(
    private readonly clipboard: ClipboardAccess,
    private readonly onIdle: () => void = () => undefined,
  ) {}

  copy(text: string, clearAfterMilliseconds: number): void {
    this.clipboard.writeText(text);
    if (this.pendingTimer !== undefined) clearTimeout(this.pendingTimer);
    this.pendingText = text;
    this.pendingTimer = setTimeout(() => this.clearIfUnchanged(), clearAfterMilliseconds);
  }

  hasPending(): boolean {
    return this.pendingTimer !== undefined;
  }

  private clearIfUnchanged(): void {
    const copiedText = this.pendingText;
    this.pendingTimer = undefined;
    this.pendingText = undefined;
    const currentText = this.clipboard.readText();
    if (currentText === undefined || currentText === copiedText) this.clipboard.writeText("");
    this.onIdle();
  }
}

export function createDocumentClipboard(document: Document): ClipboardAccess {
  return {
    writeText(text: string): void {
      const handleCopy = (event: ClipboardEvent): void => {
        event.clipboardData?.setData("text/plain", text);
        event.preventDefault();
      };
      document.addEventListener("copy", handleCopy);
      try {
        if (!document.execCommand("copy")) throw new Error("The browser refused to copy");
      } finally {
        document.removeEventListener("copy", handleCopy);
      }
    },
    readText(): string | undefined {
      const area = document.createElement("textarea");
      document.body.append(area);
      area.focus();
      try {
        return document.execCommand("paste") ? area.value : undefined;
      } finally {
        area.remove();
      }
    },
  };
}
