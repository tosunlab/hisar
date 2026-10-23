import { randomBytes } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { fillSignIn, saveScreenshot, startAdminPageFixture, type RunningFixture } from "./fixture";

let running: RunningFixture;

test.describe.configure({ mode: "serial" });

// The database may hold an earlier run's settings and invites, so the suite starts without email and uses fresh addresses.
test.beforeAll(async () => {
  running = await startAdminPageFixture();
  expect((await saveSettings({ "smtp.host": "", "smtp.username": "", invitePolicy: "anyone" })).status).toBe(200);
});

test.afterAll(() => {
  running?.fixtureProcess.kill("SIGTERM");
});

function saveSettings(settings: Record<string, unknown>): Promise<Response> {
  return fetch(`${running.fixture.baseURL}/api/v1/admin/settings`, {
    method: "PUT",
    headers: { Authorization: `Bearer ${running.fixture.token}`, "Content-Type": "application/json" },
    body: JSON.stringify({ settings }),
  });
}

const createEmail = (name: string) => `${name}-${randomBytes(4).toString("hex")}@example.com`;

async function openInvites(page: Page): Promise<void> {
  await page.goto(`${running.fixture.baseURL}/admin/invites`);
  await fillSignIn(page, running.fixture);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Invites", level: 1 })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("link", { name: "Invites" })).toHaveAttribute("aria-current", "page");
}

async function inviteToHisar(page: Page, email: string, role = "Member"): Promise<void> {
  await page.getByRole("button", { name: "Invite to Hisar…" }).click();
  const dialog = page.getByRole("dialog", { name: "Invite to Hisar" });
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByLabel(role, { exact: true }).check();
  if (role === "Admin") {
    await dialog.getByLabel("Your Secret Key").fill(running.fixture.secretKey);
    await dialog.getByLabel("Your master password").fill(running.fixture.password);
  }
  await dialog.getByRole("button", { name: "Create invite" }).click();
}

const findRow = (page: Page, email: string) => page.locator(`tr[data-invite="${email}"]`);
const readLink = (page: Page) => page.getByLabel("Invite link").inputValue();

async function fetchInvitePage(link: string): Promise<{ status: number; body: string }> {
  const response = await fetch(link);
  return { status: response.status, body: await response.text() };
}

test("creates a member invite whose link opens the invite page, then revokes it", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openInvites(page);
  const email = createEmail("newcomer");
  await inviteToHisar(page, email);
  await expect(page.getByRole("status").first()).toHaveText(
    `Email is not set up on the server, so nobody was emailed. Copy the link and send it to ${email} yourself.`,
  );
  await expect(findRow(page, email)).toContainText("Member");
  await expect(findRow(page, email)).toContainText(running.fixture.name);
  const link = await readLink(page);
  await page.getByRole("button", { name: "Copy link" }).click();
  await expect(page.getByText("Copied ·")).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(link);
  await saveScreenshot(page, "a3-created");

  const invitePage = await page.context().newPage();
  await invitePage.goto(link);
  await expect(invitePage.getByRole("heading", { name: `${running.fixture.name} invited you to Hisar` })).toBeVisible();
  await expect(invitePage.getByText(email)).toBeVisible();
  await saveScreenshot(invitePage, "a3-invite-page");

  await findRow(page, email).getByRole("button", { name: "Revoke" }).click();
  await expect(findRow(page, email)).toHaveCount(0);
  const revoked = await fetchInvitePage(link);
  const unknown = await fetchInvitePage(`${running.fixture.baseURL}/invite/never-issued-token`);
  expect(revoked.status).toBe(404);
  expect(revoked.body).toBe(unknown.body);
  await invitePage.goto(link);
  await expect(invitePage.getByRole("heading", { name: "This invite isn't valid" })).toBeVisible();
  await saveScreenshot(invitePage, "a3-revoked");
  await saveScreenshot(page, "a3-revoked-list");
});

test("resends an invite: the old link stops working and the new one opens the invite page", async ({ page }) => {
  await openInvites(page);
  const email = createEmail("resend");
  await inviteToHisar(page, email, "Admin");
  const oldLink = await readLink(page);
  await findRow(page, email).getByRole("button", { name: "Resend" }).click();
  const dialog = page.getByRole("dialog", { name: "Resend admin invite" });
  expect((await fetchInvitePage(oldLink)).status).toBe(200);
  await dialog.getByLabel("Your Secret Key").fill(running.fixture.secretKey);
  await dialog.getByLabel("Your master password").fill(running.fixture.password);
  await dialog.getByRole("button", { name: "Resend invite" }).click();
  await expect.poll(() => readLink(page)).not.toBe(oldLink);
  const newLink = await readLink(page);
  await expect(findRow(page, email)).toHaveCount(1);
  await expect(findRow(page, email)).toContainText("Admin");
  expect((await fetchInvitePage(oldLink)).status).toBe(404);
  expect((await fetchInvitePage(newLink)).status).toBe(200);
  await saveScreenshot(page, "a3-resent");
});

test("refuses bearer-only admin issuance and asks for the master password", async ({ page }) => {
  const email = createEmail("proof");
  const response = await fetch(`${running.fixture.baseURL}/api/v1/admin/invites`, {
    method: "POST",
    headers: { Authorization: `Bearer ${running.fixture.token}`, "Content-Type": "application/json" },
    body: JSON.stringify({ email, role: "admin" }),
  });
  expect(response.status).toBe(403);
  await openInvites(page);
  await page.getByRole("button", { name: "Invite to Hisar…" }).click();
  const dialog = page.getByRole("dialog", { name: "Invite to Hisar" });
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByLabel("Admin", { exact: true }).check();
  await dialog.getByLabel("Your Secret Key").fill(running.fixture.secretKey);
  await dialog.getByLabel("Your master password").fill("not the master password");
  await dialog.getByRole("button", { name: "Create invite" }).click();
  await expect(dialog.getByRole("alert")).toHaveText("incorrect email, master password or Secret Key", { timeout: 30_000 });
  await expect(findRow(page, email)).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});

test("refuses an email that already has an account", async ({ page }) => {
  await openInvites(page);
  await inviteToHisar(page, running.fixture.members[0].email);
  await expect(page.getByRole("dialog", { name: "Invite to Hisar" }).getByRole("alert")).toHaveText("An account with this email already exists.");
  await saveScreenshot(page, "a3-existing");
});

test("emails the link it shows, and lists a member's invite with the member as sender", async ({ page }) => {
  const sinkPort = Number(running.fixture.smtpSink.split(":").at(-1));
  expect((await saveSettings({ "smtp.host": "127.0.0.1", "smtp.port": sinkPort, "smtp.from": "Hisar <noreply@example.com>" })).status).toBe(200);
  await openInvites(page);
  const email = createEmail("emailed");
  await inviteToHisar(page, email);
  await expect(page.getByRole("status").first()).toHaveText(`Hisar emailed the invite to ${email}.`);
  const link = await readLink(page);
  await expect.poll(() => running.receivedEmails.filter((received) => received.recipients.includes(email)).length).toBe(1);
  expect(running.receivedEmails.find((received) => received.recipients.includes(email))!.data).toContain(link);
  await saveScreenshot(page, "a3-email");

  const member = running.fixture.members[0];
  const memberEmail = createEmail("from-member");
  const memberInvite = await fetch(`${running.fixture.baseURL}/api/v1/invites`, {
    method: "POST",
    headers: { Authorization: `Bearer ${member.token}`, "Content-Type": "application/json" },
    body: JSON.stringify({ email: memberEmail }),
  });
  expect(memberInvite.status).toBe(202);
  await expect.poll(() => running.receivedEmails.some((received) => received.recipients.includes(memberEmail))).toBe(true);
  await page.reload();
  await expect(findRow(page, memberEmail)).toContainText(member.name);
  await saveScreenshot(page, "a3-member-sent");
  expect((await saveSettings({ "smtp.host": "" })).status).toBe(200);
});
