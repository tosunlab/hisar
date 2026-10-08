import { describe, expect, it } from "vitest";
import { computePasswordStrength, getDomain, getInitials, getTileColor } from "./util";

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
    expect(getTileColor("GitHub")).toBe(getTileColor("github"));
    expect(getTileColor("GitHub")).toMatch(/^#[0-9a-f]{6}$/);
  });
});
