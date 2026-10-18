import { describe, expect, it } from "vitest";
import { findSection, sections } from "./sections";

describe("sections", () => {
  it("gives every section a unique id and a path under /admin", () => {
    expect(new Set(sections.map((section) => section.id)).size).toBe(sections.length);
    for (const section of sections) {
      expect(section.path === "/admin" || section.path.startsWith("/admin/")).toBe(true);
    }
  });

  it("opens Overview for /admin and for paths no other section owns", () => {
    for (const path of ["/admin", "/admin/", "/admin/unknown/page"]) {
      expect(findSection(path).id).toBe("overview");
    }
  });

  it("keeps an account's page in Accounts", () => {
    for (const path of ["/admin/accounts", "/admin/accounts/", "/admin/accounts/ABC123"]) {
      expect(findSection(path).id).toBe("accounts");
    }
  });
});
