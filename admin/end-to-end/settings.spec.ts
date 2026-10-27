import { once } from "node:events";
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { fillSignIn, saveScreenshot, startAdminPageFixture, type RunningFixture } from "./fixture";

// The last test restarts with these, so Email comes from the environment and the dashboard shows it read-only.
const smtpEnvironment = { HISAR_SMTP_HOST: "env.example.com", HISAR_SMTP_FROM: "Hisar <noreply@example.com>" };

let running: RunningFixture;
let savedSinkPort = "";

test.describe.configure({ mode: "serial" });

// The database may hold an earlier run's settings, so the suite starts from the defaults it changes.
test.beforeAll(async () => {
  running = await startAdminPageFixture();
  const reset = await fetch(`${running.fixture.baseURL}/api/v1/admin/settings`, {
    method: "PUT",
    headers: { Authorization: `Bearer ${running.fixture.token}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      settings: {
        "smtp.username": "",
        openRegistration: false,
        invitePolicy: "anyone",
        "minimumVersions.cli": "",
        sessionLifetimeDays: 30,
        authBurst: 1000,
        "retention.auditDays": 365,
      },
    }),
  });
  expect(reset.status).toBe(200);
});

test.afterAll(() => {
  running?.fixtureProcess.kill("SIGTERM");
});

async function openSettings(page: Page): Promise<void> {
  await page.goto(`${running.fixture.baseURL}/admin/settings`);
  await fillSignIn(page, running.fixture);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Settings", level: 1 })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("link", { name: "Settings" })).toHaveAttribute("aria-current", "page");
}

const findGroup = (page: Page, title: string) => page.getByRole("region", { name: title });

function checkVersion(request: APIRequestContext, version: string) {
  return request.get(`${running.fixture.baseURL}/api/v1/version-check`, { headers: { "Hisar-Platform": "linux/cli", "Hisar-Version": version } });
}

// startSignUpWithoutInvite starts a sign-up with no invite and nothing else, and answers the status and body.
async function startSignUpWithoutInvite(request: APIRequestContext): Promise<[number, unknown]> {
  const response = await request.post(`${running.fixture.baseURL}/api/v1/auth/register/start`, { data: { invite: "", email: "newcomer@example.com", accountId: "", request: "" } });
  return [response.status(), await response.json()];
}

test("saves Email, sends a test email and asks for the password again after a host change", async ({ page }) => {
  await openSettings(page);
  const email = findGroup(page, "Email");
  await expect(email.getByLabel("SMTP host")).toBeEditable();

  const sinkPort = running.fixture.smtpSink.split(":").at(-1)!;
  savedSinkPort = sinkPort;
  await email.getByLabel("SMTP host").fill("127.0.0.1");
  await email.getByLabel("Port").fill(sinkPort);
  await email.getByLabel("Sender").fill("Hisar <noreply@example.com>");
  await expect(email.getByText("Unsaved changes")).toBeVisible();
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByText("Saved", { exact: true })).toBeVisible();

  await email.getByRole("button", { name: "Send test email…" }).click();
  await email.getByLabel("Send to").fill("operator@example.com");
  await email.getByRole("button", { name: "Send", exact: true }).click();
  await expect(email.getByRole("alert")).toHaveText("Test email sent to operator@example.com.");
  await expect.poll(() => running.receivedEmails.filter((received) => received.recipients.includes("operator@example.com")).length).toBe(1);
  expect(running.receivedEmails.at(-1)!.data).toContain("Test email from Hisar");
  await saveScreenshot(page, "a4-test-email");

  await email.getByLabel("Port").fill("1");
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByText("Saved", { exact: true })).toBeVisible();
  await email.getByRole("button", { name: "Send", exact: true }).click();
  await expect(email.getByRole("alert")).toContainText("The test email was not sent: mail: connect:");
  await saveScreenshot(page, "a4-smtp-error");
  await email.getByLabel("Port").fill(sinkPort);
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByText("Saved", { exact: true })).toBeVisible();
  await email.getByRole("button", { name: "Close" }).click();

  await email.getByLabel("Username").fill("hisar");
  await email.getByLabel("Password").fill("smtp secret");
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByText("Saved", { exact: true })).toBeVisible();
  await expect(email.getByLabel("Password")).toHaveAttribute("placeholder", "Saved; type to replace");
  await email.getByLabel("SMTP host").fill("localhost");
  await expect(email.getByText("Enter the password again: changing the host, port or username clears the saved one.")).toBeVisible();
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByRole("alert")).toHaveText(
    "Enter the SMTP password for the username. It must be entered again whenever the host, port or username changes.",
  );
  await saveScreenshot(page, "a4-password-again");
  await email.getByLabel("SMTP host").fill("127.0.0.1");
  await email.getByLabel("Username").fill("");
  await email.getByRole("button", { name: "Save" }).click();
  await expect(email.getByText("Saved", { exact: true })).toBeVisible();
  await expect(email.getByLabel("Password")).not.toHaveAttribute("placeholder", "Saved; type to replace");
});

test("saves Sign-up, App versions and Security, and each applies without a restart", async ({ page, request }) => {
  await openSettings(page);
  const signUp = findGroup(page, "Sign-up");
  // The server checks open registration before anything else in a sign-up, so an otherwise empty start request tells
  // whether sign-ups without an invite are refused.
  expect(await startSignUpWithoutInvite(request)).toEqual([403, { error: "Sign-ups are not open on this server. Ask an admin for an invitation." }]);
  await expect(signUp.getByLabel("Anyone can sign up")).not.toBeChecked();
  await signUp.getByLabel("Anyone can sign up").check();
  await signUp.getByLabel("Admins only").check();
  await signUp.getByRole("button", { name: "Save" }).click();
  await expect(signUp.getByText("Saved", { exact: true })).toBeVisible();
  expect(await startSignUpWithoutInvite(request)).toEqual([400, { error: "invalid account ID" }]);
  const memberInvite = await request.post(`${running.fixture.baseURL}/api/v1/invites`, {
    headers: { Authorization: `Bearer ${running.fixture.members[0].token}` },
    data: { email: "newcomer@example.com" },
  });
  expect(memberInvite.status()).toBe(403);
  expect(await memberInvite.json()).toEqual({ error: "Only admins can invite people to the Hisar server." });
  await saveScreenshot(page, "a4-invite-policy");

  expect((await checkVersion(request, "1.0.0")).status()).toBe(204);
  const appVersions = findGroup(page, "App versions");
  await appVersions.getByLabel("Command-line tool").fill("99.0.0");
  await appVersions.getByRole("button", { name: "Save" }).click();
  await expect(appVersions.getByText("Saved", { exact: true })).toBeVisible();
  expect((await checkVersion(request, "1.0.0")).status()).toBe(426);

  const security = findGroup(page, "Security");
  await security.getByLabel("Session lifetime").fill("0");
  await security.getByRole("button", { name: "Save" }).click();
  await expect(security.getByRole("alert")).toHaveText("The session lifetime must be between 1 and 365 days.");
  await security.getByLabel("Session lifetime").fill("30");
  await security.getByLabel("Keep the audit log").fill("29");
  await security.getByRole("button", { name: "Save" }).click();
  await expect(security.getByRole("alert")).toHaveText("The audit log must be kept between 30 and 3650 days.");
  await security.getByLabel("Session lifetime").fill("1");
  await security.getByLabel("Auth burst").fill("50");
  await security.getByLabel("Keep the audit log").fill("400");
  await expect(security.getByText("Unsaved changes")).toBeVisible();
  await security.getByRole("button", { name: "Save" }).click();
  await expect(security.getByText("Saved", { exact: true })).toBeVisible();
  await saveScreenshot(page, "a4-settings-saved");
});

test("keeps every saved value after a restart on the same database", async ({ page, request }) => {
  const exited = once(running.fixtureProcess, "exit");
  running.fixtureProcess.kill("SIGTERM");
  await exited;
  running = await startAdminPageFixture();
  await openSettings(page);
  await expect(findGroup(page, "Email").getByLabel("SMTP host")).toHaveValue("127.0.0.1");
  await expect(findGroup(page, "Email").getByLabel("Port")).toHaveValue(savedSinkPort);
  await expect(findGroup(page, "Sign-up").getByLabel("Anyone can sign up")).toBeChecked();
  await expect(findGroup(page, "Sign-up").getByLabel("Admins only")).toBeChecked();
  await expect(findGroup(page, "App versions").getByLabel("Command-line tool")).toHaveValue("99.0.0");
  await expect(findGroup(page, "Security").getByLabel("Session lifetime")).toHaveValue("1");
  await expect(findGroup(page, "Security").getByLabel("Auth burst")).toHaveValue("50");
  await expect(findGroup(page, "Security").getByLabel("Keep the audit log")).toHaveValue("400");
  expect((await checkVersion(request, "1.0.0")).status()).toBe(426);
  await saveScreenshot(page, "a4-persisted");
});

test("locks all of Email once a HISAR_SMTP_* variable is set", async ({ page, request }) => {
  const exited = once(running.fixtureProcess, "exit");
  running.fixtureProcess.kill("SIGTERM");
  await exited;
  running = await startAdminPageFixture(smtpEnvironment);
  await openSettings(page);
  const email = findGroup(page, "Email");
  await expect(email.getByText("Email is set by HISAR_SMTP_* variables on the server. Change them there and restart the server.")).toBeVisible();
  await expect(email.getByLabel("SMTP host")).toHaveValue("env.example.com");
  await expect(email.getByText("Set by HISAR_SMTP_HOST")).toBeVisible();
  await expect(email.getByLabel("Port")).toHaveValue("587");
  for (const label of ["SMTP host", "Port", "Username", "Password", "Sender"]) {
    await expect(email.getByLabel(label)).not.toBeEditable();
  }
  await expect(email.getByRole("button", { name: "Save" })).toBeDisabled();
  await saveScreenshot(page, "a4-settings-env-locked");
  const portChange = await request.put(`${running.fixture.baseURL}/api/v1/admin/settings`, {
    headers: { Authorization: `Bearer ${running.fixture.token}` },
    data: { settings: { "smtp.port": Number(savedSinkPort) } },
  });
  expect(portChange.status()).toBe(400);
  expect(await portChange.json()).toEqual({ error: "Email is set by HISAR_SMTP_* variables on the server, so it cannot be changed here." });
});
