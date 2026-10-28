import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { createApp, nextTick, type App } from "vue";
import SettingsView from "./SettingsView.vue";
import { isEmailFromEnvironment, listChanges, needsPasswordAgain, settingGroups, type AdminSettings } from "./settings";

function createSettings(): AdminSettings {
  return {
    "smtp.host": { value: "smtp.example.com", source: "database", environmentVariable: "HISAR_SMTP_HOST" },
    "smtp.port": { value: 587, source: "default", environmentVariable: "HISAR_SMTP_PORT" },
    "smtp.username": { value: "hisar", source: "database", environmentVariable: "HISAR_SMTP_USERNAME" },
    "smtp.password": { value: { set: true }, source: "database", environmentVariable: "HISAR_SMTP_PASSWORD" },
    "smtp.from": { value: "noreply@example.com", source: "database", environmentVariable: "HISAR_SMTP_FROM" },
    openRegistration: { value: false, source: "default", environmentVariable: "HISAR_OPEN_REGISTRATION" },
    invitePolicy: { value: "anyone", source: "default" },
    "minimumVersions.desktopMacOS": { value: "", source: "default", environmentVariable: "HISAR_MINIMUM_VERSION_DESKTOP_MACOS" },
    "minimumVersions.iOS": { value: "", source: "default", environmentVariable: "HISAR_MINIMUM_VERSION_IOS" },
    "minimumVersions.android": { value: "", source: "default", environmentVariable: "HISAR_MINIMUM_VERSION_ANDROID" },
    "minimumVersions.cli": { value: "99.0.0", source: "environment", environmentVariable: "HISAR_MINIMUM_VERSION_CLI" },
    "minimumVersions.extension": { value: "", source: "default", environmentVariable: "HISAR_MINIMUM_VERSION_EXTENSION" },
    sessionLifetimeDays: { value: 30, source: "default" },
    authBurst: { value: 10, source: "default", environmentVariable: "HISAR_AUTH_BURST" },
    "retention.historyDays": { value: 90, source: "default" },
    "retention.auditDays": { value: 365, source: "default" },
    "retention.expiredInvitesDays": { value: 30, source: "default" },
    "push.relayURL": { value: "https://push.tryhisar.com", source: "default", environmentVariable: "HISAR_PUSH_RELAY_URL" },
  };
}

// createEnvironmentSettings is what the server answers once HISAR_SMTP_HOST is set: every Email field comes from the environment.
function createEnvironmentSettings(): AdminSettings {
  const settings = createSettings();
  for (const name of ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from"]) {
    settings[name] = { ...settings[name], source: "environment" };
  }
  settings["smtp.host"].value = "env.example.com";
  settings["smtp.username"].value = "";
  settings["smtp.password"].value = { set: false };
  return settings;
}

const flush = async () => {
  for (let index = 0; index < 5; index++) {
    await Promise.resolve();
    await nextTick();
  }
};

const [email, signUp, , security] = settingGroups;
const emailForm = { "smtp.host": "smtp.example.com", "smtp.port": 587, "smtp.username": "hisar", "smtp.password": "", "smtp.from": "noreply@example.com" };

describe("listChanges", () => {
  it("leaves out fields the environment sets and an empty password", () => {
    const settings = createSettings();
    expect(listChanges(email, settings, emailForm)).toEqual({});
    expect(listChanges(email, settings, { ...emailForm, "smtp.port": 2525, "smtp.password": "new secret" })).toEqual({ "smtp.port": 2525, "smtp.password": "new secret" });
    expect(listChanges(email, createEnvironmentSettings(), { ...emailForm, "smtp.host": "attacker.example.com", "smtp.port": 2525 })).toEqual({});
  });

  it("sends numbers as numbers and leaves a non-number for the server to refuse", () => {
    const settings = createSettings();
    const form = { sessionLifetimeDays: "1", authBurst: 10, "retention.historyDays": "2.5", "retention.auditDays": 365, "retention.expiredInvitesDays": 30 };
    expect(listChanges(security, settings, form)).toEqual({ sessionLifetimeDays: 1, "retention.historyDays": "2.5" });
    expect(listChanges(signUp, settings, { openRegistration: false, invitePolicy: "admins" })).toEqual({ invitePolicy: "admins" });
  });

  it("sends a checkbox as a boolean, and never one the environment sets", () => {
    const settings = createSettings();
    expect(listChanges(signUp, settings, { openRegistration: true, invitePolicy: "anyone" })).toEqual({ openRegistration: true });
    expect(listChanges(signUp, settings, { openRegistration: false, invitePolicy: "anyone" })).toEqual({});
    settings.openRegistration = { ...settings.openRegistration, value: true, source: "database" };
    expect(listChanges(signUp, settings, { openRegistration: false, invitePolicy: "anyone" })).toEqual({ openRegistration: false });
    settings.openRegistration = { ...settings.openRegistration, source: "environment" };
    expect(listChanges(signUp, settings, { openRegistration: false, invitePolicy: "anyone" })).toEqual({});
  });
});

describe("needsPasswordAgain", () => {
  it("asks for a saved password again when the host, port or username changes", () => {
    const settings = createSettings();
    expect(needsPasswordAgain(settings, emailForm)).toBe(false);
    expect(needsPasswordAgain(settings, { ...emailForm, "smtp.from": "other@example.com" })).toBe(false);
    for (const change of [{ "smtp.host": "new.example.com" }, { "smtp.port": 2525 }, { "smtp.username": "other" }]) {
      expect(needsPasswordAgain(settings, { ...emailForm, ...change })).toBe(true);
      expect(needsPasswordAgain(settings, { ...emailForm, ...change, "smtp.password": "secret" })).toBe(false);
    }
    settings["smtp.password"].value = { set: false };
    expect(needsPasswordAgain(settings, { ...emailForm, "smtp.host": "new.example.com" })).toBe(false);
  });
});

describe("isEmailFromEnvironment", () => {
  it("is true once the server says Email comes from the environment", () => {
    expect(isEmailFromEnvironment(createSettings())).toBe(false);
    expect(isEmailFromEnvironment(createEnvironmentSettings())).toBe(true);
  });
});

describe("SettingsView", () => {
  let app: App | undefined;
  let root: HTMLElement;
  let fetchMock: Mock<typeof fetch>;

  async function mount(loaded: AdminSettings): Promise<void> {
    fetchMock = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === "PUT") {
        const settings = structuredClone(loaded);
        const { settings: changes } = JSON.parse(String(init.body)) as { settings: Record<string, unknown> };
        for (const [name, value] of Object.entries(changes)) {
          settings[name] = { ...settings[name], value, source: "database" };
        }
        return new Response(JSON.stringify({ settings }), { status: 200 });
      }
      return new Response(JSON.stringify({ settings: loaded }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    root = document.createElement("div");
    document.body.append(root);
    app = createApp(SettingsView);
    app.mount(root);
    await flush();
  }

  afterEach(() => {
    app?.unmount();
    root.remove();
    vi.unstubAllGlobals();
  });

  const findInput = (name: string) => root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
  const findGroup = (title: string) => [...root.querySelectorAll<HTMLElement>("section.group")].find((group) => group.querySelector("h2")?.textContent === title)!;
  const findSaveButton = (title: string) => [...findGroup(title).querySelectorAll("button")].find((button) => button.textContent?.trim() === "Save")!;
  const type = async (name: string, value: string) => {
    const input = findInput(name);
    input.value = value;
    input.dispatchEvent(new Event("input"));
    await flush();
  };

  it("shows a field the environment sets read-only with its variable", async () => {
    await mount(createSettings());
    expect(findInput("minimumVersions.cli").readOnly).toBe(true);
    expect(findGroup("App versions").textContent).toContain("Set by HISAR_MINIMUM_VERSION_CLI");
    expect(findInput("smtp.host").readOnly).toBe(false);
    expect(findInput("smtp.password").value).toBe("");
    expect(findInput("smtp.password").placeholder).toBe("Saved; type to replace");
    expect(findGroup("Email").textContent).not.toContain("HISAR_SMTP_");
  });

  it("locks all of Email when the environment sets it", async () => {
    await mount(createEnvironmentSettings());
    for (const name of ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from"]) {
      expect(findInput(name).readOnly).toBe(true);
    }
    expect(findInput("smtp.host").value).toBe("env.example.com");
    expect(findGroup("Email").textContent).toContain("Email is set by HISAR_SMTP_* variables on the server.");
    expect(findGroup("Email").textContent).toContain("Set by HISAR_SMTP_HOST");
    expect(findSaveButton("Email").disabled).toBe(true);
    expect([...findGroup("Email").querySelectorAll("button")].some((button) => button.textContent === "Send test email…")).toBe(true);
  });

  it("says the password must be entered again when the host changes", async () => {
    await mount(createSettings());
    const passwordAgain = "Enter the password again: changing the host, port or username clears the saved one.";
    expect(findGroup("Email").textContent).not.toContain(passwordAgain);
    await type("smtp.host", "new.example.com");
    expect(findGroup("Email").textContent).toContain(passwordAgain);
    await type("smtp.password", "secret");
    expect(findGroup("Email").textContent).not.toContain(passwordAgain);
  });

  it("saves open registration from its checkbox and locks it when the environment sets it", async () => {
    await mount(createSettings());
    const checkbox = findInput("openRegistration");
    expect(checkbox.type).toBe("checkbox");
    expect(checkbox.checked).toBe(false);
    expect(checkbox.disabled).toBe(false);
    expect(findGroup("Sign-up").textContent).toContain("Anyone can sign up");
    checkbox.click();
    await flush();
    expect(findGroup("Sign-up").textContent).toContain("Unsaved changes");
    findGroup("Sign-up").querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    const [, saveRequest] = fetchMock.mock.calls.at(-1)! as [string, RequestInit];
    expect(JSON.parse(String(saveRequest.body))).toEqual({ settings: { openRegistration: true } });
    expect(findInput("openRegistration").checked).toBe(true);
    expect(findGroup("Sign-up").textContent).toContain("Saved");
  });

  it("shows open registration the environment sets as a disabled checkbox with its variable", async () => {
    const settings = createSettings();
    settings.openRegistration = { ...settings.openRegistration, value: true, source: "environment" };
    await mount(settings);
    expect(findInput("openRegistration").checked).toBe(true);
    expect(findInput("openRegistration").disabled).toBe(true);
    expect(findGroup("Sign-up").textContent).toContain("Set by HISAR_OPEN_REGISTRATION");
  });

  it("marks a group with unsaved changes until it is saved or reverted", async () => {
    await mount(createSettings());
    expect(findSaveButton("Security").disabled).toBe(true);
    expect(findGroup("Security").textContent).not.toContain("Unsaved changes");

    await type("sessionLifetimeDays", "1");
    expect(findGroup("Security").textContent).toContain("Unsaved changes");
    expect(findGroup("Email").textContent).not.toContain("Unsaved changes");
    expect(findSaveButton("Security").disabled).toBe(false);

    await type("sessionLifetimeDays", "30");
    expect(findGroup("Security").textContent).not.toContain("Unsaved changes");

    await type("sessionLifetimeDays", "1");
    await type("smtp.port", "2525");
    findGroup("Security").querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    const [, saveRequest] = fetchMock.mock.calls.at(-1)! as [string, RequestInit];
    expect(saveRequest.method).toBe("PUT");
    expect(JSON.parse(String(saveRequest.body))).toEqual({ settings: { sessionLifetimeDays: 1 } });
    expect(findGroup("Security").textContent).not.toContain("Unsaved changes");
    expect(findGroup("Security").textContent).toContain("Saved");
    expect(findGroup("Email").textContent).toContain("Unsaved changes");
    expect(findInput("smtp.port").value).toBe("2525");
  });
});
