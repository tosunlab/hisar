import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp } from "vue";
import type { AdminHealth } from "../../api";
import OverviewView from "./OverviewView.vue";

const health: AdminHealth = {
  version: "9.9.9",
  latestVersion: "10.0.0",
  database: {
    ok: true,
    migrations: [
      { name: "00001_init.sql", appliedAt: "2026-10-17T12:00:00Z" },
      { name: "00002_more.sql", appliedAt: "2026-10-17T12:00:00Z" },
    ],
  },
  counts: { accounts: 3, vaults: 5, items: 1234, shareLinks: 1, pendingInvites: 2 },
  storageBytes: 48_213_000,
  checks: [
    { id: "public-url", title: "Public URL", status: "failed", message: 'https://hisar.example.com/healthz answered "healthy" instead of ok, so it does not lead to this server.' },
    { id: "https", title: "HTTPS", status: "ok", message: "HISAR_PUBLIC_URL uses https." },
    { id: "email", title: "Email", status: "warning", message: "Email is not set up. Invites from the apps will fail." },
  ],
};

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

// mountOverview answers each request with answerPath and waits until Overview shows the health or an error.
async function mountOverview(answerPath: (path: string) => Response): Promise<HTMLElement> {
  vi.stubGlobal("fetch", vi.fn(async (path: string) => answerPath(path)));
  const root = document.createElement("div");
  createApp(OverviewView).mount(root);
  await vi.waitFor(() => {
    if (readText(root, "version") === "…" && !root.querySelector("[role=alert]")) {
      throw new Error("Overview is still loading");
    }
  });
  return root;
}

function readText(root: HTMLElement, testID: string): string | undefined {
  return root.querySelector(`[data-testid="${testID}"]`)?.textContent?.trim();
}

describe("Overview", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows the version, an available update, the counts, the database and every check", async () => {
    const root = await mountOverview((path) => (path === "/api/v1/admin/server" ? answer(200, { publicUrl: "https://hisar.example.com" }) : answer(200, health)));
    expect(readText(root, "public-url")).toBe("https://hisar.example.com");
    expect(readText(root, "version")).toBe("9.9.9");
    expect(readText(root, "update")).toBe("Version 10.0.0 is available");
    expect(readText(root, "database")).toBe("48.2 MB · All 2 migrations applied");
    expect(readText(root, "count-items")).toBe("1,234Items");
    expect(readText(root, "count-share-links")).toBe("1Share link");
    expect(readText(root, "count-pending-invites")).toBe("2Pending invites");
    expect(readText(root, "counts-hint")).toBe("The item count may be up to a minute old.");
    const allCheck = [...root.querySelectorAll<HTMLElement>("[data-testid^=check-]")].map((row) => [
      row.dataset.status,
      row.querySelector("[role=img]")?.getAttribute("aria-label"),
      row.querySelector(".check-title")?.textContent,
    ]);
    expect(allCheck).toEqual([
      ["failed", "Failed", "Public URL"],
      ["ok", "OK", "HTTPS"],
      ["warning", "Warning", "Email"],
    ]);
    expect(readText(root, "check-public-url")).toContain("does not lead to this server");
  });

  it("shows no update when the server runs the newest release", async () => {
    const root = await mountOverview((path) => (path === "/api/v1/admin/server" ? answer(200, { publicUrl: "" }) : answer(200, { ...health, latestVersion: "" })));
    expect(readText(root, "version")).toBe("9.9.9");
    expect(root.querySelector("[data-testid=update]")).toBeNull();
  });

  it("shows the server's error when the database is down, and the server it reached", async () => {
    const root = await mountOverview((path) =>
      path === "/api/v1/admin/server" ? answer(200, { publicUrl: "https://hisar.example.com" }) : answer(503, { error: "the server cannot reach its database" }),
    );
    await vi.waitFor(() => expect(readText(root, "public-url")).toBe("https://hisar.example.com"));
    expect(root.querySelector("[role=alert]")?.textContent).toBe("the server cannot reach its database");
    expect(root.querySelector("[data-testid^=check-]")).toBeNull();
    expect(root.querySelector("[data-testid^=count-]")).toBeNull();
  });

  it("writes the database size in the next unit once it rounds to 1,000", async () => {
    const root = await mountOverview((path) => (path === "/api/v1/admin/server" ? answer(200, { publicUrl: "" }) : answer(200, { ...health, storageBytes: 999_960 })));
    expect(readText(root, "database")).toBe("1 MB · All 2 migrations applied");
  });

  it("names the pending migrations' count when one is not applied", async () => {
    const pending = { ok: false, migrations: [health.database.migrations[0], { name: "00003_new.sql", appliedAt: null }] };
    const root = await mountOverview((path) => (path === "/api/v1/admin/server" ? answer(200, { publicUrl: "" }) : answer(200, { ...health, database: pending })));
    expect(readText(root, "database")).toBe("48.2 MB · 1 of 2 migrations applied");
  });

  it("records the secrets backup with I have a backup and shows the check ok", async () => {
    const backupCheck = { id: "secrets-backup", title: "Secrets backup", status: "warning", message: "Back up hisar-secrets.json. Nobody can sign in without it." };
    const confirmedCheck = { ...backupCheck, status: "ok", message: "An admin confirmed a backup of hisar-secrets.json on 2026-10-17." };
    const allRequest: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        allRequest.push(`${init?.method ?? "GET"} ${path}`);
        if (path === "/api/v1/admin/server") {
          return answer(200, { publicUrl: "" });
        }
        return path === "/api/v1/admin/health/secrets-backup" ? answer(200, confirmedCheck) : answer(200, { ...health, checks: [health.checks[0], backupCheck] });
      }),
    );
    const root = document.createElement("div");
    createApp(OverviewView).mount(root);
    await vi.waitFor(() => expect(root.querySelector("[data-testid=check-secrets-backup] button")?.textContent?.trim()).toBe("I have a backup"));
    root.querySelector<HTMLButtonElement>("[data-testid=check-secrets-backup] button")!.click();
    await vi.waitFor(() => expect(root.querySelector("[data-testid=check-secrets-backup]")?.getAttribute("data-status")).toBe("ok"));
    expect(root.querySelector("[data-testid=check-secrets-backup] button")).toBeNull();
    expect(readText(root, "check-secrets-backup")).toContain("confirmed a backup");
    expect(root.querySelector("[data-testid=check-public-url]")?.getAttribute("data-status")).toBe("failed");
    expect(allRequest.filter((request) => request.endsWith("/admin/health"))).toHaveLength(1);
    expect(allRequest).toContain("POST /api/v1/admin/health/secrets-backup");
  });

  it("keeps I have a backup and shows the error when the server refuses it", async () => {
    const backupCheck = { id: "secrets-backup", title: "Secrets backup", status: "warning", message: "Back up hisar-secrets.json. Nobody can sign in without it." };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        if (path === "/api/v1/admin/health/secrets-backup") {
          return answer(503, { error: "the server cannot reach its database" });
        }
        return path === "/api/v1/admin/server" ? answer(200, { publicUrl: "" }) : answer(200, { ...health, checks: [backupCheck] });
      }),
    );
    const root = document.createElement("div");
    createApp(OverviewView).mount(root);
    await vi.waitFor(() => expect(root.querySelector("[data-testid=check-secrets-backup] button")).not.toBeNull());
    root.querySelector<HTMLButtonElement>("[data-testid=check-secrets-backup] button")!.click();
    await vi.waitFor(() => expect(root.querySelector("[data-testid=check-secrets-backup] [role=alert]")?.textContent).toBe("the server cannot reach its database"));
    expect(root.querySelector("[data-testid=check-secrets-backup]")?.getAttribute("data-status")).toBe("warning");
    expect(root.querySelector<HTMLButtonElement>("[data-testid=check-secrets-backup] button")?.disabled).toBe(false);
  });
});
