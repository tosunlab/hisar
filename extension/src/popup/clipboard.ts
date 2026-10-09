export async function writeToClipboard(text: string): Promise<void> {
  await navigator.clipboard.writeText(text);
}
