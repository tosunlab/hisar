import { afterEach, describe, expect, it, vi } from "vitest";
import { auth, notAdminMessage, signIn } from "./api";
import { bridge } from "./bridge/bridge";

describe("signIn", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("refuses a member with the refusal even when revoking the member's device fails", async () => {
    vi.spyOn(bridge, "signIn").mockResolvedValue({ token: "member-token", accountId: "account", deviceId: "device", role: "member", name: "Member" });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ error: "internal error" }), { status: 500 }));

    await expect(signIn("member@example.com", "H1-A", "password")).rejects.toThrow(notAdminMessage);
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/devices/device", { method: "DELETE", headers: { Authorization: "Bearer member-token" } });
    expect(auth.session).toBeNull();
  });
});
