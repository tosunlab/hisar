import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { fillSignIn, saveScreenshot, startAdminPageFixture, type RunningFixture } from "./fixture";

let running: RunningFixture;

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  running = await startAdminPageFixture();
});

test.afterAll(() => {
  running?.fixtureProcess.kill("SIGTERM");
});

async function openAuditLog(page: Page): Promise<void> {
  await page.goto(`${running.fixture.baseURL}/admin/audit`);
  await fillSignIn(page, running.fixture);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Audit log", level: 1 })).toBeVisible({ timeout: 30_000 });
}

// countCSVRecords counts the records after the header, minding quoted fields that hold line breaks.
function countCSVRecords(text: string): number {
  let recordCount = 0;
  let isQuoted = false;
  for (const character of text) {
    if (character === '"') {
      isQuoted = !isQuoted;
    } else if (character === "\n" && !isQuoted) {
      recordCount++;
    }
  }
  return recordCount - 1;
}

test("shows the sign-ups and the admin's own sign-in, filters by a member and exports what it shows", async ({ page }) => {
  const { fixture } = running;
  const [member] = fixture.members;
  await openAuditLog(page);
  const rows = page.locator("tbody tr");
  await expect(rows.first()).toHaveAttribute("data-type", "login");
  await expect(rows.first().locator("td.account")).toHaveText(fixture.email);
  for (const account of [fixture, ...fixture.members]) {
    await expect(page.locator("tbody tr[data-type=signup]").filter({ hasText: account.email })).toHaveCount(1);
  }
  await saveScreenshot(page, "a5-audit-log");

  const accountFilter = page.getByRole("searchbox", { name: "Account" });
  await accountFilter.fill(member.email);
  await accountFilter.press("Enter");
  await expect(rows.first().locator("td.account")).toHaveText(member.email);
  const shownCount = await rows.count();
  expect(await page.locator("td.account", { hasText: member.email }).count()).toBe(shownCount);
  await saveScreenshot(page, "a5-member-filter");

  const downloadEvent = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export CSV" }).click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toMatch(/^hisar-audit-\d{4}-\d{2}-\d{2}\.csv$/);
  expect(countCSVRecords(await readFile(await download.path(), "utf8"))).toBe(shownCount);

  await accountFilter.fill("");
  await accountFilter.press("Enter");
  await page.getByRole("combobox", { name: "Type" }).selectOption({ label: "All admin actions" });
  await expect(rows.first()).toHaveAttribute("data-type", "admin_audit_exported");
  await saveScreenshot(page, "a5-admin-actions");
});
