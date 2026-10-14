// themes.json defines tile0 to tile9 with their text colours for every theme.
const tileColorCount = 10;

export interface TileColor {
  background: string;
  color: string;
}

export function getDomain(link: string): string {
  try {
    return new URL(link.includes("://") ? link : `https://${link}`).hostname.replace(/^www\./, "");
  } catch {
    return link;
  }
}

export function getTileColor(text: string): TileColor {
  let hash = 0;
  for (const character of text.toLowerCase()) {
    hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
  }
  const tileIndex = hash % tileColorCount;
  return { background: `var(--color-tile${tileIndex})`, color: `var(--color-tile${tileIndex}-text)` };
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
export const minimumMasterPasswordLength = 10;
// Below this only clearly guessable passwords are refused; the meter still says Weak up to 40 bits.
const minimumMasterPasswordBits = 28;
const masterPasswordAdvice = "A few random words you can remember make a strong master password.";

export interface MasterPasswordFeedback {
  message: string;
  isBlocking: boolean;
}

export function getMasterPasswordFeedback(password: string): MasterPasswordFeedback {
  if (password === "") {
    return { message: masterPasswordAdvice, isBlocking: false };
  }
  if (password.length < minimumMasterPasswordLength) {
    return { message: `Use at least ${minimumMasterPasswordLength} characters.`, isBlocking: true };
  }
  const strength = computePasswordStrength(password);
  if (strength.bits < minimumMasterPasswordBits) {
    return { message: "This password is too easy to guess. Add a few more words or characters.", isBlocking: true };
  }
  if (strength.level === "weak") {
    return { message: "You can use it, but another word or a few more characters make it much harder to guess.", isBlocking: false };
  }
  return { message: masterPasswordAdvice, isBlocking: false };
}

export function describeSignedOutDevices(signedOutDeviceCount: number): string {
  if (signedOutDeviceCount === 0) {
    return "No other devices were signed in.";
  }
  if (signedOutDeviceCount === 1) {
    return "1 other device was signed out.";
  }
  return `${signedOutDeviceCount} other devices were signed out.`;
}

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

export const gitHubSSHKeyURL = "https://github.com/settings/ssh/new";
export const gitLabSSHKeyURL = "https://gitlab.com/-/user_settings/ssh_keys";

export interface CommitSigningCommand {
  description: string;
  command: string;
}

// buildCommitSigningCommandList sets up git to sign with this key through the agent; the key loses its comment,
// so only base64 and the key type end up inside the quotes.
export function buildCommitSigningCommandList(publicKey: string): CommitSigningCommand[] {
  const [keyType = "", keyData = ""] = publicKey.trim().split(/\s+/);
  if (!/^[a-z0-9@.-]+$/.test(keyType) || !/^[A-Za-z0-9+/=]+$/.test(keyData)) {
    return [];
  }
  const key = `${keyType} ${keyData}`;
  return [
    { description: "Sign with SSH keys", command: "git config --global gpg.format ssh" },
    { description: "Use this key", command: `git config --global user.signingkey "key::${key}"` },
    { description: "Sign every commit", command: "git config --global commit.gpgsign true" },
    {
      description: "Trust your own signatures",
      command: `mkdir -p ~/.ssh && echo "$(git config --get user.email) namespaces=\\"git\\" ${key}" >> ~/.ssh/allowed_signers`,
    },
    { description: "Check signatures with that file", command: "git config --global gpg.ssh.allowedSignersFile ~/.ssh/allowed_signers" },
  ];
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

// formatTimeAgo is a short "since" for banners: "just now", "5 min ago", "2 h ago", "yesterday", "3 days ago".
export function formatTimeAgo(timestamp: string, now = new Date()): string {
  const minuteCount = Math.floor((now.getTime() - new Date(timestamp).getTime()) / 60000);
  if (Number.isNaN(minuteCount)) {
    return "";
  }
  if (minuteCount < 1) {
    return "just now";
  }
  if (minuteCount < 60) {
    return `${minuteCount} min ago`;
  }
  const hourCount = Math.floor(minuteCount / 60);
  if (hourCount < 24) {
    return `${hourCount} h ago`;
  }
  const dayCount = Math.floor(hourCount / 24);
  return dayCount === 1 ? "yesterday" : `${dayCount} days ago`;
}

export function describePerson(name: string, email: string): string {
  return name && name !== email ? name : email;
}

export const allShareLinkExpiry = [
  { seconds: 3600, label: "1 hour" },
  { seconds: 86400, label: "1 day" },
  { seconds: 604800, label: "7 days" },
  { seconds: 2592000, label: "30 days" },
];

export function formatShareLinkViews(maxViews: number, viewCount: number): string {
  if (maxViews === 0) {
    return viewCount === 1 ? "1 view · unlimited" : `${viewCount} views · unlimited`;
  }
  return `${viewCount} of ${maxViews} ${maxViews === 1 ? "view" : "views"}`;
}

export function formatViewLimit(maxViews: number): string {
  if (maxViews === 0) {
    return "unlimited views";
  }
  return maxViews === 1 ? "1 view" : `${maxViews} views`;
}

export function formatExpiresIn(timestamp: string, now = new Date()): string {
  const minuteCount = Math.ceil((new Date(timestamp).getTime() - now.getTime()) / 60000);
  if (Number.isNaN(minuteCount)) {
    return "";
  }
  if (minuteCount <= 0) {
    return "Expired";
  }
  if (minuteCount < 60) {
    return minuteCount === 1 ? "Expires in 1 minute" : `Expires in ${minuteCount} minutes`;
  }
  const hourCount = Math.round(minuteCount / 60);
  if (hourCount < 24) {
    return hourCount === 1 ? "Expires in 1 hour" : `Expires in ${hourCount} hours`;
  }
  const dayCount = Math.round(hourCount / 24);
  return dayCount === 1 ? "Expires in 1 day" : `Expires in ${dayCount} days`;
}

export function formatDateTime(timestamp: string): string {
  return new Date(timestamp).toLocaleString(undefined, { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function describeShareLinkContent(shareLink: { isText: boolean; fieldLabels: string[] }): string {
  if (shareLink.isText) {
    return "Text";
  }
  const description = shareLink.fieldLabels.join(", ");
  return description.charAt(0).toUpperCase() + description.slice(1);
}
