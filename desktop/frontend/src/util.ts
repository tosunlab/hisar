const allTileColor = ["#2c3e50", "#3b3f46", "#1f4e4a", "#4a3b2a", "#4b2d34", "#2d3a52", "#3a4a2c", "#463355", "#2b4657", "#52402b"];

export function getDomain(link: string): string {
  try {
    return new URL(link.includes("://") ? link : `https://${link}`).hostname.replace(/^www\./, "");
  } catch {
    return link;
  }
}

export function getTileColor(text: string): string {
  let hash = 0;
  for (const character of text.toLowerCase()) {
    hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
  }
  return allTileColor[hash % allTileColor.length];
}

export function getInitials(text: string): string {
  const allWord = text.trim().split(/[\s@._-]+/).filter((word) => word !== "");
  if (allWord.length === 0) {
    return "?";
  }
  if (allWord.length === 1) {
    return allWord[0][0].toUpperCase();
  }
  return (allWord[0][0] + allWord[1][0]).toUpperCase();
}

export type StrengthLevel = "empty" | "weak" | "fair" | "good" | "strong";

export interface PasswordStrength {
  level: StrengthLevel;
  label: string;
  bits: number;
}

const allCommonPattern = [
  /pass(word)?/i,
  /qwert/i,
  /admin/i,
  /welcome/i,
  /letmein/i,
  /iloveyou/i,
  /(spring|summer|autumn|fall|winter)/i,
  /(19|20)\d\d/,
  /1234/,
  /abc/i,
];

const bitsPerDictionaryWord = 11;

// computePasswordStrength estimates entropy: dictionary-style phrases count per word, common
// patterns are penalized, otherwise character classes times length, discounting repeats and sequences.
export function computePasswordStrength(password: string): PasswordStrength {
  if (password === "") {
    return { level: "empty", label: "", bits: 0 };
  }
  const penalty = allCommonPattern.filter((pattern) => pattern.test(password)).length * 20;
  if (/^[a-z]+([-_ .][a-z]+)+$/i.test(password)) {
    const wordCount = password.split(/[-_ .]/).length;
    return classifyBits(wordCount * bitsPerDictionaryWord - penalty);
  }
  let poolSize = 0;
  if (/[a-z]/.test(password)) poolSize += 26;
  if (/[A-Z]/.test(password)) poolSize += 26;
  if (/[0-9]/.test(password)) poolSize += 10;
  if (/[^a-zA-Z0-9]/.test(password)) poolSize += 33;

  let effectiveLength = 0;
  for (let index = 0; index < password.length; index++) {
    const previous = password.charCodeAt(index - 1);
    const current = password.charCodeAt(index);
    const isRepeat = index > 0 && current === previous;
    const isSequence = index > 0 && Math.abs(current - previous) === 1;
    effectiveLength += isRepeat || isSequence ? 0.3 : 1;
  }
  return classifyBits(Math.round(effectiveLength * Math.log2(Math.max(poolSize, 2))) - penalty);
}

function classifyBits(estimatedBits: number): PasswordStrength {
  const bits = Math.max(estimatedBits, 0);
  if (bits < 40) return { level: "weak", label: "Weak", bits };
  if (bits < 60) return { level: "fair", label: "Fair", bits };
  if (bits < 90) return { level: "good", label: "Good", bits };
  return { level: "strong", label: "Strong", bits };
}

export function isMacPlatform(): boolean {
  return /Mac/i.test(navigator.platform || navigator.userAgent);
}

export const sshKeyCategory = "ssh-key";

const allKeyTypeLabel: Record<string, string> = {
  "ssh-ed25519": "Ed25519",
  "ssh-rsa": "RSA",
  "ecdsa-sha2-nistp256": "ECDSA P-256",
  "ecdsa-sha2-nistp384": "ECDSA P-384",
  "ecdsa-sha2-nistp521": "ECDSA P-521",
};

export function formatKeyType(keyType: string): string {
  return allKeyTypeLabel[keyType] ?? keyType;
}

export function formatRole(role: string): string {
  return role ? role[0].toUpperCase() + role.slice(1) : "";
}

const allOperatingSystemLabel: Record<string, string> = { darwin: "macOS", linux: "Linux", windows: "Windows", ios: "iOS", android: "Android" };
const allAppLabel: Record<string, string> = { desktop: "app", cli: "command line", extension: "browser extension", mobile: "app" };

// formatPlatform turns a device platform such as "darwin/desktop" into "macOS app".
export function formatPlatform(platform: string): string {
  const [operatingSystem, app] = platform.split("/");
  const operatingSystemLabel = allOperatingSystemLabel[operatingSystem] ?? operatingSystem;
  return app ? `${operatingSystemLabel} ${allAppLabel[app] ?? app}` : operatingSystemLabel;
}

export function getPlatformIcon(platform: string): "laptop" | "phone" | "terminal" {
  if (platform.endsWith("/cli")) {
    return "terminal";
  }
  return /^(ios|android)\//.test(platform) ? "phone" : "laptop";
}

export function formatLastSeen(timestamp: string, now = new Date()): string {
  const seenAt = new Date(timestamp);
  const minuteCount = Math.floor((now.getTime() - seenAt.getTime()) / 60000);
  if (Number.isNaN(minuteCount)) {
    return "";
  }
  if (minuteCount < 2) {
    return "Active now";
  }
  if (minuteCount < 60) {
    return `${minuteCount} minutes ago`;
  }
  const hourCount = Math.floor(minuteCount / 60);
  if (hourCount < 24) {
    return hourCount === 1 ? "1 hour ago" : `${hourCount} hours ago`;
  }
  const dayCount = Math.floor(hourCount / 24);
  if (dayCount < 30) {
    return dayCount === 1 ? "Yesterday" : `${dayCount} days ago`;
  }
  return seenAt.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

export function formatSubmittedTime(timestamp: string, now = new Date()): string {
  const lastSeen = formatLastSeen(timestamp, now);
  if (lastSeen === "Active now") {
    return "just now";
  }
  return lastSeen === "Yesterday" ? "yesterday" : lastSeen;
}

export function formatVaultCount(count: number): string {
  return count === 1 ? "1 vault" : `${count} vaults`;
}
