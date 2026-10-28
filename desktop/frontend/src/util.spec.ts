import { describe, expect, it } from "vitest";
import {
  buildCommitSigningCommandList,
  customFieldNameWarning,
  describeSignedOutDevices,
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
import {
  describeAPICredentialExpiry,
  describePerson,
  formatCardNumberInput,
  formatCardSummary,
  formatExpiryInput,
  formatInputAsTyped,
  formatTimeAgo,
  isCalendarDate,
} from "./util";

describe("custom field reference names", () => {
  it("warns for duplicate and built-in names without changing their values", () => {
    const fields = [{ name: "Recovery code", value: "first" }, { name: "recovery CODE", value: "second" }, { name: "Password", value: "custom" }];
    expect(customFieldNameWarning(fields, 0)).toBe("Secret references need a unique name.");
    expect(customFieldNameWarning(fields, 1)).toBe("Secret references need a unique name.");
    expect(customFieldNameWarning(fields, 2)).toBe("A secret reference with this name reaches the built-in field.");
    expect(fields.map((field) => field.value)).toEqual(["first", "second", "custom"]);
  });
});

describe("describeSignedOutDevices", () => {
  it("does not report an unknown count as zero", () => {
    expect(describeSignedOutDevices(-1)).toBe("Any other devices were signed out; their count could not be recovered.");
    expect(describeSignedOutDevices(0)).toBe("No other devices were signed in.");
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

describe("buildCommitSigningCommandList", () => {
  it("drops the key comment and refuses anything that is not a public key", () => {
    const allCommand = buildCommitSigningCommandList('ssh-rsa AAAAB3NzaC1yc2E= deniz "quoted" $(whoami)').map((entry) => entry.command);
    expect(allCommand[1]).toBe('git config --global user.signingkey "key::ssh-rsa AAAAB3NzaC1yc2E="');
    expect(allCommand.join("\n")).not.toContain("whoami");
    expect(buildCommitSigningCommandList("")).toEqual([]);
    expect(buildCommitSigningCommandList("ssh-ed25519 $(rm -rf ~)")).toEqual([]);
    expect(buildCommitSigningCommandList('ssh"ed AAAA')).toEqual([]);
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

describe("formatCardSummary", () => {
  it("prefers the card's type to the brand of its number", () => {
    expect(formatCardSummary({ type: "Troy", brand: "Visa", last4: "1111" })).toBe("Troy •••• 1111");
    expect(formatCardSummary({ type: "", brand: "Visa", last4: "1111" })).toBe("Visa •••• 1111");
    expect(formatCardSummary({ type: "", brand: "", last4: "" })).toBe("Credit card");
  });
});

describe("card input formatting", () => {
  it("groups a card number as it is typed", () => {
    expect(formatCardNumberInput("4444555555555555")).toBe("4444 5555 5555 5555");
    expect(formatCardNumberInput("44445")).toBe("4444 5");
    expect(formatCardNumberInput("4444 ")).toBe("4444");
    expect(formatCardNumberInput("4111-1111-1111-1111")).toBe("4111 1111 1111 1111");
    expect(formatCardNumberInput("378282246310005")).toBe("3782 822463 10005");
    expect(formatCardNumberInput("36227206271667")).toBe("3622 720627 1667");
    expect(formatCardNumberInput("3056930009020004")).toBe("3056 9300 0902 0004");
    expect(formatCardNumberInput("4111111111111111111")).toBe("4111 1111 1111 1111 111");
    expect(formatCardNumberInput("Store card #12")).toBe("Store card #12");
  });

  it("writes an expiry as MM/YY as it is typed", () => {
    const cases: Record<string, string> = {
      "0": "0",
      "01": "01",
      "013": "01/3",
      "0130": "01/30",
      "01305": "01/30",
      "3": "03",
      "13": "01/3",
      "1/": "01/",
      "01/": "01/",
      "1/30": "01/30",
      "12/2030": "12/30",
      "2030-12": "12/30",
      "122030": "12/30",
      "": "",
      "soon": "soon",
    };
    for (const [typed, want] of Object.entries(cases)) {
      expect(formatExpiryInput(typed), typed).toBe(want);
    }
  });

  it("keeps the caret after the same digits and deletes through a separator", () => {
    const input = document.createElement("input");
    input.value = "41111";
    expect(formatInputAsTyped(input, formatCardNumberInput, "4111", "insertText")).toBe("4111 1");
    expect(input.selectionStart).toBe(6);

    input.value = "4111 9 1111";
    input.setSelectionRange(6, 6);
    expect(formatInputAsTyped(input, formatCardNumberInput, "4111 1111", "insertText")).toBe("4111 9111 1");
    expect(input.selectionStart).toBe(6);

    input.value = "41111111";
    input.setSelectionRange(4, 4);
    expect(formatInputAsTyped(input, formatCardNumberInput, "4111 1111", "deleteContentBackward")).toBe("4111 111");
    expect(input.selectionStart).toBe(3);
  });
});

describe("isCalendarDate", () => {
  it("accepts only days that exist, written YYYY-MM-DD", () => {
    expect(isCalendarDate("1990-04-12")).toBe(true);
    expect(isCalendarDate("2024-02-29")).toBe(true);
    expect(isCalendarDate("2023-02-29")).toBe(false);
    expect(isCalendarDate("2023-02-31")).toBe(false);
    expect(isCalendarDate("1990-13-01")).toBe(false);
    expect(isCalendarDate("1990-4-12")).toBe(false);
    expect(isCalendarDate("04/12/1990")).toBe(false);
  });
});

describe("describeAPICredentialExpiry", () => {
  it("warns from the day after the expiry, in local time", () => {
    const evening = new Date(2030, 0, 15, 23, 30);
    expect(describeAPICredentialExpiry("2030-01-14", evening)).toBe("This credential has expired");
    expect(describeAPICredentialExpiry("2030-01-15", evening)).toBe("");
    expect(describeAPICredentialExpiry("2030-01-14", new Date(2030, 0, 15))).toBe("This credential has expired");
    expect(describeAPICredentialExpiry("2030-01-14", new Date(2030, 0, 14, 23, 59))).toBe("");
  });

  it("does not warn for a value that is not a date", () => {
    expect(describeAPICredentialExpiry("", new Date(2030, 0, 15))).toBe("");
    expect(describeAPICredentialExpiry("never", new Date(2030, 0, 15))).toBe("");
    expect(describeAPICredentialExpiry("2020-02-31", new Date(2030, 0, 15))).toBe("");
  });
});
