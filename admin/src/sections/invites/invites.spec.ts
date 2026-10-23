import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { createApp, nextTick, type App } from "vue";
import { auth } from "../../api";
import { bridge } from "../../bridge/bridge";
import InvitesView from "./InvitesView.vue";
import { describeDelivery, describeSender, formatDate, type AdminInvite, type CreatedInvite } from "./invites";

vi.mock("../../bridge/bridge", () => ({ bridge: { reauthenticate: vi.fn() } }));
const reauthenticate = bridge.reauthenticate as Mock<typeof bridge.reauthenticate>;
const proof = { reauthenticationId: "fresh", ke3: "a2Uz" };

const fromMember: AdminInvite = {
  id: "aa11",
  email: "carol@example.com",
  role: "member",
  createdBy: { accountId: "MEMBER", name: "Ada Lovelace", email: "ada@example.com" },
  createdAt: "2026-10-17T09:00:00Z",
  expiresAt: "2026-10-24T09:00:00Z",
};

const fromOperator: AdminInvite = { ...fromMember, id: "bb22", email: "dave@example.com", role: "admin", createdBy: null };

function createCreated(invite: AdminInvite, overrides: Partial<CreatedInvite> = {}): CreatedInvite {
  return { invite, link: `https://hisar.example.com/invite/token-${invite.id}`, emailed: false, ...overrides };
}

const flush = async () => {
  for (let index = 0; index < 5; index++) {
    await Promise.resolve();
    await nextTick();
  }
};

describe("invites", () => {
  it("names the sender, or hisar-server invite for the operator's invites", () => {
    expect(describeSender(fromMember)).toBe("Ada Lovelace");
    expect(describeSender({ ...fromMember, createdBy: { accountId: "MEMBER", name: " ", email: "ada@example.com" } })).toBe("ada@example.com");
    expect(describeSender(fromOperator)).toBe("hisar-server invite");
  });

  it("writes dates as the invite email does", () => {
    expect(formatDate("2026-10-24T09:00:00Z")).toBe("24 October 2026");
  });

  it("says whether the invite was emailed", () => {
    expect(describeDelivery(createCreated(fromMember, { emailed: true }))).toBe("Hisar emailed the invite to carol@example.com.");
    expect(describeDelivery(createCreated(fromMember))).toContain("Email is not set up on the server");
    expect(describeDelivery(createCreated(fromMember, { emailError: "535 authentication failed" }))).toBe(
      "The email was not sent: 535 authentication failed. Copy the link and send it to carol@example.com yourself.",
    );
  });
});

describe("InvitesView", () => {
  let app: App | undefined;
  let root: HTMLElement;
  let fetchMock: Mock<typeof fetch>;
  let writeText: Mock<(text: string) => Promise<void>>;

  async function mount(listed: AdminInvite[], answers: Record<string, () => Response> = {}): Promise<void> {
    auth.session = { token: "admin-token", accountId: "ADMIN1", deviceId: "device", name: "Admin" };
    reauthenticate.mockResolvedValue(proof);
    fetchMock = vi.fn<typeof fetch>(async (url, init) => {
      const answer = answers[`${init?.method ?? "GET"} ${String(url)}`];
      return answer ? answer() : new Response(JSON.stringify(String(url) === "/api/v1/account" ? { email: "admin@example.com" } : { invites: listed }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    writeText = vi.fn(async () => undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    root = document.createElement("div");
    document.body.append(root);
    app = createApp(InvitesView);
    app.mount(root);
    await flush();
  }

  afterEach(() => {
    app?.unmount();
    root.remove();
    vi.unstubAllGlobals();
    auth.session = null;
    reauthenticate.mockReset();
  });

  const findButton = (text: string, scope: ParentNode = root) => [...scope.querySelectorAll("button")].find((button) => button.textContent?.trim() === text)!;
  const findRows = () => [...root.querySelectorAll<HTMLTableRowElement>("tbody tr")];
  const click = async (button: HTMLButtonElement) => {
    button.click();
    await flush();
  };

  it("says so when no invite is pending", async () => {
    await mount([]);
    expect(root.querySelector("[data-testid=no-invites]")?.textContent).toBe("No pending invites.");
    expect(root.querySelector("table")).toBeNull();
  });

  it("lists pending invites with their sender", async () => {
    await mount([fromMember, fromOperator]);
    expect(findRows().map((row) => row.dataset.invite)).toEqual(["carol@example.com", "dave@example.com"]);
    expect(findRows()[0].textContent).toContain("Ada Lovelace");
    expect(findRows()[1].textContent).toContain("Admin");
    expect(findRows()[1].textContent).toContain("hisar-server invite");
  });

  it("creates an invite, shows its link once and copies it", async () => {
    const fresh = { ...fromMember, id: "cc33", email: "erin@example.com" };
    await mount([fromMember], { "POST /api/v1/admin/invites": () => new Response(JSON.stringify(createCreated(fresh)), { status: 201 }) });
    await click(findButton("Invite to Hisar…"));
    const email = root.querySelector<HTMLInputElement>("input[name=email]")!;
    email.value = "erin@example.com";
    email.dispatchEvent(new Event("input"));
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();

    const [, createRequest] = fetchMock.mock.calls.at(-1)! as [string, RequestInit];
    expect(JSON.parse(String(createRequest.body))).toEqual({ email: "erin@example.com", role: "member" });
    expect(root.querySelector("form")).toBeNull();
    expect(findRows().map((row) => row.dataset.invite)).toEqual(["erin@example.com", "carol@example.com"]);
    const created = root.querySelector<HTMLElement>("[data-testid=created-invite]")!;
    expect(created.querySelector<HTMLInputElement>("input[name=link]")!.value).toBe("https://hisar.example.com/invite/token-cc33");
    expect(created.textContent).not.toContain("Copied");

    await click(findButton("Copy link", created));
    expect(writeText).toHaveBeenCalledWith("https://hisar.example.com/invite/token-cc33");
    expect(created.textContent).toContain("Copied");

    await click(findButton("Done", created));
    expect(root.querySelector("[data-testid=created-invite]")).toBeNull();
  });

  it("asks to select the link when the clipboard refuses", async () => {
    await mount([], { "POST /api/v1/admin/invites": () => new Response(JSON.stringify(createCreated(fromMember)), { status: 201 }) });
    writeText.mockRejectedValue(new Error("denied"));
    await click(findButton("Invite to Hisar…"));
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    await click(findButton("Copy link"));
    expect(root.querySelector("[data-testid=created-invite]")!.textContent).toContain("Select the link and copy it.");
  });

  it("keeps the form open with the server's refusal", async () => {
    await mount([], {
      "POST /api/v1/admin/invites": () => new Response(JSON.stringify({ error: "An account with this email already exists." }), { status: 409 }),
    });
    await click(findButton("Invite to Hisar…"));
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    expect(root.querySelector("form [role=alert]")?.textContent).toBe("An account with this email already exists.");
    expect(root.querySelector("[data-testid=created-invite]")).toBeNull();
  });

  it("revokes a row and replaces a resent one with its new link", async () => {
    const resent = { ...fromOperator, id: "dd44" };
    await mount([fromMember, fromOperator], {
      "DELETE /api/v1/admin/invites/aa11": () => new Response(null, { status: 204 }),
      "POST /api/v1/admin/invites/bb22/resend": () => new Response(JSON.stringify(createCreated(resent, { emailed: true })), { status: 201 }),
    });
    await click(findButton("Revoke", findRows()[0]));
    expect(findRows().map((row) => row.dataset.invite)).toEqual(["dave@example.com"]);

    await click(findButton("Resend", findRows()[0]));
    expect(root.querySelector("[data-testid=created-invite]")).toBeNull();
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await vi.waitFor(() => expect(root.querySelector("[data-testid=created-invite]")).not.toBeNull());
    expect(reauthenticate).toHaveBeenCalledOnce();
    const [, resendRequest] = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/resend"))! as [string, RequestInit];
    expect(JSON.parse(String(resendRequest.body))).toEqual({ proof });
    expect(findRows()).toHaveLength(1);
    const created = root.querySelector<HTMLElement>("[data-testid=created-invite]")!;
    expect(created.querySelector<HTMLInputElement>("input[name=link]")!.value).toBe("https://hisar.example.com/invite/token-dd44");
    expect(created.textContent).toContain("Hisar emailed the invite to dave@example.com.");
  });

  it("proves admin creation and refuses to issue when the factors fail", async () => {
    const fresh = { ...fromOperator, id: "ee55" };
    await mount([], { "POST /api/v1/admin/invites": () => new Response(JSON.stringify(createCreated(fresh)), { status: 201 }) });
    await click(findButton("Invite to Hisar…"));
    root.querySelector<HTMLInputElement>("input[name=role][value=admin]")!.click();
    await flush();
    for (const [name, value] of [["secretKey", "H1-ADMIN1-SECRET"], ["password", "admin password"]]) {
      const input = root.querySelector<HTMLInputElement>(`input[name=${name}]`)!;
      input.value = value!;
      input.dispatchEvent(new Event("input"));
    }
    reauthenticate.mockRejectedValueOnce(new Error("incorrect master password"));
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await vi.waitFor(() => expect(root.querySelector("[role=alert]")?.textContent).toBe("incorrect master password"));
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await vi.waitFor(() => expect(root.querySelector("form")).toBeNull());
    const [, createRequest] = fetchMock.mock.calls.find(([, init]) => init?.method === "POST")! as [string, RequestInit];
    expect(JSON.parse(String(createRequest.body))).toEqual({ email: "", role: "admin", proof });
    expect(reauthenticate).toHaveBeenLastCalledWith("admin-token", "admin@example.com", "H1-ADMIN1-SECRET", "admin password");
  });
});
