import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { createApp, nextTick, type App } from "vue";
import AuditView from "./AuditView.vue";
import { buildQuery, describeDetail, type AuditEvent, type AuditPage } from "./audit";

const groups = [
  { id: "accounts", label: "Accounts and sign-in", types: [{ type: "login", label: "Signed in" }, { type: "reauthentication_failed", label: "Master password confirmation failed" }] },
  { id: "admin", label: "Admin actions", types: [{ type: "admin_settings_changed", label: "Settings changed" }] },
];

function createEvent(id: number, overrides: Partial<AuditEvent> = {}): AuditEvent {
  return {
    id,
    accountId: "AAAAAA",
    accountEmail: "member@example.com",
    deviceId: null,
    type: "login",
    detail: { device: "Mac", platform: "macos" },
    ip: "192.0.2.1",
    createdAt: "2026-10-18T09:00:00Z",
    ...overrides,
  };
}

const flush = async () => {
  for (let index = 0; index < 5; index++) {
    await Promise.resolve();
    await nextTick();
  }
};

describe("buildQuery", () => {
  it("sends only the filters that are set, the range as UTC times and the cursor last", () => {
    expect(buildQuery({ account: " ", kind: "", from: "", to: "" })).toBe("");
    const from = "2026-10-18T09:00";
    const query = new URLSearchParams(buildQuery({ account: " member@example.com ", kind: "group:admin", from, to: "" }, "123.4").slice(1));
    expect([...query.keys()]).toEqual(["account", "group", "from", "cursor"]);
    expect(query.get("account")).toBe("member@example.com");
    expect(query.get("from")).toBe(new Date(from).toISOString());
    expect(buildQuery({ account: "", kind: "type:reauthentication_failed", from: "", to: "" })).toBe("?type=reauthentication_failed");
  });
});

describe("describeDetail", () => {
  it("writes name and value pairs", () => {
    expect(describeDetail(null)).toBe("");
    expect(describeDetail({ fields: ["invitePolicy", "authBurst"], deletedVaults: 2 })).toBe("fields: invitePolicy, authBurst · deletedVaults: 2");
  });
});

describe("AuditView", () => {
  let app: App | undefined;
  let root: HTMLElement;
  let fetchMock: Mock<typeof fetch>;

  async function mount(pages: Record<string, AuditPage>): Promise<void> {
    fetchMock = vi.fn<typeof fetch>(async (url) => {
      const path = String(url);
      if (path === "/api/v1/admin/audit/types") {
        return new Response(JSON.stringify({ groups }), { status: 200 });
      }
      const query = new URLSearchParams(path.split("?")[1] ?? "");
      return new Response(JSON.stringify(pages[query.get("cursor") ?? query.get("type") ?? ""] ?? { events: [], nextCursor: "" }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    root = document.createElement("div");
    document.body.append(root);
    app = createApp(AuditView);
    app.mount(root);
    await flush();
  }

  afterEach(() => {
    app?.unmount();
    root.remove();
    vi.unstubAllGlobals();
  });

  const listRequestedQueries = () =>
    fetchMock.mock.calls.map(([url]) => String(url)).filter((url) => url.startsWith("/api/v1/admin/audit?") || url === "/api/v1/admin/audit");
  const findButton = (text: string) => [...root.querySelectorAll("button")].find((button) => button.textContent?.trim() === text);

  it("loads the next page with the cursor and starts again without it when a filter changes", async () => {
    await mount({
      "": { events: [createEvent(3), createEvent(2)], nextCursor: "page-2" },
      "page-2": { events: [createEvent(1)], nextCursor: "" },
      reauthentication_failed: { events: [createEvent(9, { type: "reauthentication_failed" })], nextCursor: "" },
    });
    expect(root.querySelectorAll("tbody tr")).toHaveLength(2);
    findButton("Load more")!.click();
    await flush();
    expect(root.querySelectorAll("tbody tr")).toHaveLength(3);
    expect(findButton("Load more")).toBeUndefined();

    const select = root.querySelector<HTMLSelectElement>('select[name="type"]')!;
    expect([...select.options].map((option) => option.textContent)).toContain("All admin actions");
    select.value = "type:reauthentication_failed";
    select.dispatchEvent(new Event("change"));
    await flush();
    expect(listRequestedQueries()).toEqual(["/api/v1/admin/audit", "/api/v1/admin/audit?cursor=page-2", "/api/v1/admin/audit?type=reauthentication_failed"]);
    expect(root.querySelectorAll("tbody tr")).toHaveLength(1);
    expect(root.querySelector("tbody")!.textContent).toContain("Master password confirmation failed");
  });

  it("shows a deleted account by its ID", async () => {
    await mount({ "": { events: [createEvent(1, { accountEmail: null, type: "account_deleted", detail: { deletedVaults: 1 } })], nextCursor: "" } });
    const account = root.querySelector("td.account")!.textContent;
    expect(account).toContain("AAAAAA");
    expect(account).toContain("Deleted account");
    expect(root.querySelector("tbody")!.textContent).toContain("account_deleted");
  });
});
