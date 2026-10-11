import { describe, expect, it } from "vitest";
import {
  computePasswordStrength,
  formatExpiresIn,
  formatShareLinkViews,
  formatViewLimit,
  formatKeyType,
  formatLastSeen,
  formatPlatform,
  formatRole,
  getDomain,
  getInitials,
  getPlatformIcon,
  getTileColor,
} from "./util";
import { describePerson, formatTimeAgo } from "./util";

describe("computePasswordStrength", () => {
  it("rates random passwords by length and character classes", () => {
    expect(computePasswordStrength("").level).toBe("empty");
    expect(computePasswordStrength("k7#Vq2!mZp9@Lx4$Rt8w").level).toBe("strong");
    expect(computePasswordStrength("Mx9$Lp2!Qw7@").level).toBe("good");
    expect(computePasswordStrength("abcdef").level).toBe("weak");
  });

  it("treats word phrases as dictionary words, not random characters", () => {
    expect(computePasswordStrength("netflix-family-pass").level).toBe("weak");
    expect(computePasswordStrength("correct-horse-battery-staple-orbit-lantern-mosaic-tundra").level).toBe("good");
  });

  it("penalizes common patterns", () => {
    expect(computePasswordStrength("password1").level).toBe("weak");
    expect(computePasswordStrength("Spring2024!").level).toBe("weak");
    expect(computePasswordStrength("aaaaaaaaaaaaaaaa").level).toBe("weak");
  });
});

describe("getInitials", () => {
  it("uses one letter for one word and two for several", () => {
    expect(getInitials("GitHub")).toBe("G");
    expect(getInitials("AWS Console")).toBe("AC");
    expect(getInitials("alper@tryhisar.com")).toBe("AT");
    expect(getInitials("  ")).toBe("?");
  });
});

describe("getDomain", () => {
  it("returns the host without www", () => {
    expect(getDomain("https://www.isbank.com.tr/login")).toBe("isbank.com.tr");
    expect(getDomain("github.com")).toBe("github.com");
    expect(getDomain("not a url")).toBe("not a url");
  });
});

describe("getTileColor", () => {
  it("is stable per title and case-insensitive", () => {
    expect(getTileColor("GitHub")).toEqual(getTileColor("github"));
    expect(getTileColor("GitHub").background).toMatch(/^var\(--color-tile\d\)$/);
    expect(getTileColor("GitHub").color).toBe(getTileColor("GitHub").background.replace(")", "-text)"));
  });
});

describe("formatKeyType", () => {
  it("names common SSH key types and keeps unknown ones", () => {
    expect(formatKeyType("ssh-ed25519")).toBe("Ed25519");
    expect(formatKeyType("ssh-rsa")).toBe("RSA");
    expect(formatKeyType("ecdsa-sha2-nistp256")).toBe("ECDSA P-256");
    expect(formatKeyType("sk-ssh-ed25519@openssh.com")).toBe("sk-ssh-ed25519@openssh.com");
  });
});

describe("device and role formatting", () => {
  it("names platforms and picks an icon", () => {
    expect(formatPlatform("darwin/desktop")).toBe("macOS app");
    expect(formatPlatform("linux/cli")).toBe("Linux command line");
    expect(formatPlatform("plan9")).toBe("plan9");
    expect(getPlatformIcon("linux/cli")).toBe("terminal");
    expect(getPlatformIcon("ios/mobile")).toBe("phone");
    expect(getPlatformIcon("darwin/desktop")).toBe("laptop");
    expect(formatRole("editor")).toBe("Editor");
  });

  it("describes when a device was last seen", () => {
    const now = new Date("2026-10-09T12:00:00Z");
    expect(formatLastSeen("2026-10-09T11:59:30Z", now)).toBe("Active now");
    expect(formatLastSeen("2026-10-09T11:35:00Z", now)).toBe("25 minutes ago");
    expect(formatLastSeen("2026-10-09T11:00:00Z", now)).toBe("1 hour ago");
    expect(formatLastSeen("2026-10-08T09:00:00Z", now)).toBe("Yesterday");
    expect(formatLastSeen("2026-10-01T12:00:00Z", now)).toBe("8 days ago");
    expect(formatLastSeen("not a date", now)).toBe("");
  });
});

describe("share link formatting", () => {
  it("describes views used against the limit", () => {
    expect(formatShareLinkViews(1, 0)).toBe("0 of 1 view");
    expect(formatShareLinkViews(5, 2)).toBe("2 of 5 views");
    expect(formatShareLinkViews(0, 1)).toBe("1 view · unlimited");
    expect(formatViewLimit(0)).toBe("unlimited views");
    expect(formatViewLimit(1)).toBe("1 view");
  });

  it("describes the time until expiry", () => {
    const now = new Date("2026-10-11T12:00:00Z");
    expect(formatExpiresIn("2026-10-11T12:30:00Z", now)).toBe("Expires in 30 minutes");
    expect(formatExpiresIn("2026-10-11T13:00:00Z", now)).toBe("Expires in 1 hour");
    expect(formatExpiresIn("2026-10-18T12:00:00Z", now)).toBe("Expires in 7 days");
    expect(formatExpiresIn("2026-10-11T11:00:00Z", now)).toBe("Expired");
  });
});

describe("banner formatting", () => {
  it("says how long ago something happened in short units", () => {
    const now = new Date("2026-10-11T12:00:00Z");
    expect(formatTimeAgo("2026-10-11T11:59:40Z", now)).toBe("just now");
    expect(formatTimeAgo("2026-10-11T11:55:00Z", now)).toBe("5 min ago");
    expect(formatTimeAgo("2026-10-11T10:00:00Z", now)).toBe("2 h ago");
    expect(formatTimeAgo("2026-10-10T09:00:00Z", now)).toBe("yesterday");
    expect(formatTimeAgo("2026-10-08T12:00:00Z", now)).toBe("3 days ago");
    expect(formatTimeAgo("", now)).toBe("");
  });

  it("names a person by name, or by email when the name is missing", () => {
    expect(describePerson("Ada", "ada@example.com")).toBe("Ada");
    expect(describePerson("", "ada@example.com")).toBe("ada@example.com");
    expect(describePerson("ada@example.com", "ada@example.com")).toBe("ada@example.com");
  });
});
