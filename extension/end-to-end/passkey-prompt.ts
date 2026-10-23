import { expect, type Frame, type Page } from "@playwright/test";

export function findPromptFrame(page: Page): Frame | undefined {
  return page.frames().find((frame) => frame.url().includes("/passkey-prompt.html"));
}

export async function waitForPrompt(page: Page): Promise<Frame> {
  await expect.poll(() => findPromptFrame(page) !== undefined, { timeout: 25_000 }).toBe(true);
  return findPromptFrame(page)!;
}

export async function resetStatus(page: Page): Promise<void> {
  await page.locator("#status").evaluate((element) => (element.textContent = "ready"));
}

export async function readStatus(page: Page): Promise<Record<string, unknown>> {
  await expect(page.locator("#status")).not.toHaveText(/^(ready|waiting for autofill)$/);
  return JSON.parse((await page.locator("#status").textContent()) ?? "{}") as Record<string, unknown>;
}

// The prompt ignores clicks for its first 300 ms on screen, so the click repeats until the request ends and the frame goes;
// a real click would never settle once its frame is removed, hence a trial click and then the mouse.
export async function clickInPrompt(page: Page, prompt: Frame, testID: string): Promise<void> {
  await expect(async () => {
    if (!prompt.isDetached()) {
      const target = prompt.getByTestId(testID).first();
      await target.click({ trial: true, timeout: 2_000 });
      const box = await target.boundingBox({ timeout: 1_000 });
      if (box) void page.mouse.click(box.x + box.width / 2, box.y + box.height / 2).catch(() => undefined);
    }
    await expect.poll(() => prompt.isDetached(), { timeout: 2_000 }).toBe(true);
  }).toPass({ timeout: 30_000 });
}
