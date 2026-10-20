import type { IconName } from "./components/AppIcon.vue";

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

// thisDevice is how the UI names the computer it runs on: "this Mac", or "this computer" on Linux.
export function getThisDeviceName(): string {
  return isMacPlatform() ? "this Mac" : "this computer";
}

// getTrayName is where Hisar stays while its window is closed.
export function getTrayName(): string {
  return isMacPlatform() ? "menu bar" : "system tray";
}

export const sshKeyCategory = "ssh-key";
export const secureNoteCategory = "secure-note";
export const creditCardCategory = "credit-card";
export const identityCategory = "identity";
export const apiCredentialCategory = "api-credential";
export const addressCategory = "address";
export const softwareLicenseCategory = "software-license";
export const serverCategory = "server";
export const databaseCategory = "database";

type CategoryIcon = "key" | "note" | "card" | "identity" | "apiKey" | "address" | "license" | "server" | "database";
const allCategoryIcon: Record<string, CategoryIcon> = {
  [sshKeyCategory]: "key",
  [secureNoteCategory]: "note",
  [creditCardCategory]: "card",
  [identityCategory]: "identity",
  [apiCredentialCategory]: "apiKey",
  [addressCategory]: "address",
  [softwareLicenseCategory]: "license",
  [serverCategory]: "server",
  [databaseCategory]: "database",
};

// getItemIcon is the tile icon of an item's category; logins show their initials instead.
export function getItemIcon(category: string): CategoryIcon | undefined {
  return allCategoryIcon[category];
}

// The names of a card's custom fields, as core/item stores them; copying and secret references use them.
export const cardFieldName = {
  cardholder: "Cardholder",
  type: "Type",
  number: "Card number",
  expiry: "Expiry",
  securityCode: "Security code",
  pin: "PIN",
  // billingAddress is the one-line address a card had before it kept one in the address fields.
  billingAddress: "Billing address",
} as const;

// The names of an address's custom fields, as core/item stores them; identities and cards keep their address in the same
// fields, and a hisar:// reference names them too.
export const addressFieldName = {
  street: "Street",
  city: "City",
  region: "Region",
  postalCode: "Postal code",
  country: "Country",
} as const;

export type AddressKey = keyof typeof addressFieldName;
export const allAddressKey = Object.keys(addressFieldName) as AddressKey[];

// formatAddressSummary is an address's line in lists: its city and country, or the first line of its street, such as
// "Springfield, US".
export function formatAddressSummary(address: { street: string; city: string; country: string } | null | undefined): string {
  const cityAndCountry = [address?.city, address?.country].filter((part) => part).join(", ");
  return cityAndCountry || (address?.street ?? "").split("\n")[0] || "Address";
}

// The names of an identity's custom fields, as core/item stores them.
export const identityFieldName = {
  firstName: "First name",
  middleName: "Middle name",
  lastName: "Last name",
  birthDate: "Birth date",
  email: "Email",
  phone: "Phone",
  // address is the one-line address an identity had before it kept one in the address fields.
  address: "Address",
  passportNumber: "Passport number",
  driversLicenseNumber: "Driver's license number",
  nationalIdNumber: "National ID number",
} as const;

// The names of an API credential's custom fields, as core/item stores them; a hisar:// reference names them too.
export const apiCredentialFieldName = {
  service: "Service",
  keyId: "Key ID",
  secret: "Secret",
  host: "Host",
  expiry: "Expiry",
} as const;

// formatAPICredentialSummary is an API credential's line in lists: its service and host, such as "Stripe · api.stripe.com".
export function formatAPICredentialSummary(credential: { service: string; host: string } | null | undefined): string {
  return [credential?.service, credential?.host].filter((part) => part).join(" · ") || "API credential";
}

// The names of a software license's custom fields, as core/item stores them; a hisar:// reference names them too.
export const softwareLicenseFieldName = {
  product: "Product",
  version: "Version",
  licenseKey: "License key",
  licensedTo: "Licensed to",
  email: "Email",
  purchaseDate: "Purchase date",
  downloadUrl: "Download URL",
} as const;

// formatSoftwareLicenseSummary is a software license's line in lists: its version and who it is licensed to, such as
// "Version 100 · Alice Example", or its email or product when it has neither.
export function formatSoftwareLicenseSummary(license: { product: string; version: string; licensedTo: string; email: string } | null | undefined): string {
  const version = license?.version ? (/^\d/.test(license.version) ? `Version ${license.version}` : license.version) : "";
  const summary = [version, license?.licensedTo || license?.email || ""].filter((part) => part).join(" · ");
  return summary || license?.product || "Software license";
}

// The names of a server's custom fields, as core/item stores them; its username, password and admin console are the
// item's own.
export const serverFieldName = {
  host: "Host",
  port: "Port",
  sshKey: "SSH key",
} as const;

// formatServerAddress joins a server's host and port as core/item does, with brackets around an IPv6 host.
export function formatServerAddress(server: { host: string; port: string } | null | undefined): string {
  const host = server?.host ?? "";
  if (!host || !server?.port) {
    return host;
  }
  return `${host.includes(":") ? `[${host}]` : host}:${server.port}`;
}

// formatServerSummary is a server's line in lists: its host and port and its username, such as
// "build.example.com:2222 · deploy".
export function formatServerSummary(server: { host: string; port: string } | null | undefined, username: string): string {
  return [formatServerAddress(server), username].filter((part) => part).join(" · ") || "Server";
}

// describeServerPort warns about a port that is not a number from 1 to 65535, as core/item does; "" for an empty port.
export function describeServerPort(port: string): string {
  if (port === "") {
    return "";
  }
  const number = Number(port);
  return /^\d+$/.test(port) && number >= 1 && number <= 65535 ? "" : "The port is not a number from 1 to 65535";
}

// The names of a database's custom fields, as core/item stores them; its username and password are the item's own.
export const databaseFieldName = {
  engine: "Engine",
  host: "Host",
  port: "Port",
  name: "Database",
  options: "Options",
} as const;

// allDatabaseEngine are the engines the editor offers and Hisar builds connection strings for, as core/item lists them; any
// other engine is stored as typed.
export const allDatabaseEngine = [
  { id: "postgresql", name: "PostgreSQL" },
  { id: "mysql", name: "MySQL" },
  { id: "mariadb", name: "MariaDB" },
  { id: "sqlserver", name: "SQL Server" },
  { id: "oracle", name: "Oracle" },
  { id: "mongodb", name: "MongoDB" },
  { id: "redis", name: "Redis" },
] as const;

// isKnownDatabaseEngine tells whether an engine is one of allDatabaseEngine by its ID.
export function isKnownDatabaseEngine(engine: string): boolean {
  return allDatabaseEngine.some((known) => known.id === engine);
}

// formatDatabaseLocation is a database's address and name, such as "db.example.com:5432/app".
export function formatDatabaseLocation(database: { host: string; port: string; name: string } | null | undefined): string {
  const address = formatServerAddress(database);
  if (!database?.name) {
    return address;
  }
  return address ? `${address}/${database.name}` : database.name;
}

// formatDatabaseSummary is a database's line in lists: its engine, address and name, such as
// "PostgreSQL · db.example.com:5432/app".
export function formatDatabaseSummary(database: { engineName: string; host: string; port: string; name: string } | null | undefined): string {
  return [database?.engineName ?? "", formatDatabaseLocation(database)].filter((part) => part).join(" · ") || "Database";
}

type IdentityName = { firstName: string; middleName: string; lastName: string; email?: string };

// formatFullName joins the name parts that are set, such as "Alice M. Example".
export function formatFullName(identity: IdentityName | null | undefined): string {
  return [identity?.firstName, identity?.middleName, identity?.lastName].filter((part) => part).join(" ");
}

// formatIdentitySummary is an identity's line in lists: the full name, or the email when it has no name.
export function formatIdentitySummary(identity: IdentityName | null | undefined): string {
  return formatFullName(identity) || identity?.email || "Identity";
}

// isCalendarDate tells whether date is a day that exists written as YYYY-MM-DD, the only value a date input shows; it
// rejects 2023-02-31.
export function isCalendarDate(date: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
    return false;
  }
  const [year, month, day] = date.split("-").map(Number);
  const parsed = new Date(Date.UTC(year, month - 1, day));
  return parsed.getUTCFullYear() === year && parsed.getUTCMonth() === month - 1 && parsed.getUTCDate() === day;
}

// describeAPICredentialExpiry warns once a YYYY-MM-DD expiry has passed in the local time zone, as core/item
// DescribeAPICredentialExpiry does; it returns "" for any other value.
export function describeAPICredentialExpiry(expiry: string, now = new Date()): string {
  if (!isCalendarDate(expiry)) {
    return "";
  }
  const [year, month, day] = expiry.split("-").map(Number);
  return now >= new Date(year, month - 1, day + 1) ? "This credential has expired" : "";
}

// The brands an editor offers as a card's type, in the order of core/item ListCardBrands.
export const allCardBrand = ["Visa", "Mastercard", "American Express", "Discover", "Diners Club", "JCB", "UnionPay", "Maestro", "Troy"];

// formatCardSummary is a card's line in lists: its type (or the brand its number points to) and last four digits, such as
// "Visa •••• 4242".
export function formatCardSummary(card: { type: string; brand: string; last4: string } | null | undefined): string {
  const last4 = card?.last4 ? `•••• ${card.last4}` : "";
  return [card?.type || card?.brand || "", last4].filter((part) => part !== "").join(" ") || "Credit card";
}

// formatCardNumberInput groups a card number as it is typed, the way core/item saves it: 4-6-5 for American Express,
// 4-6-4 for a 14-digit Diners Club number and fours otherwise. Text with other characters is left as typed.
export function formatCardNumberInput(value: string): string {
  if (!/^[\d -]*$/.test(value)) {
    return value;
  }
  const digits = value.replace(/\D/g, "");
  let groupSizes = [4];
  if (/^3[47]/.test(digits) && digits.length <= 15) {
    groupSizes = [4, 6, 5];
  } else if (/^3(0[0-5]|[689])/.test(digits) && digits.length <= 14) {
    groupSizes = [4, 6, 4];
  }
  const groups: string[] = [];
  for (let start = 0, index = 0; start < digits.length; index++) {
    const size = groupSizes[Math.min(index, groupSizes.length - 1)];
    groups.push(digits.slice(start, start + size));
    start += size;
  }
  return groups.join(" ");
}

// formatExpiryInput writes an expiry as MM/YY while it is typed: "1230" becomes "12/30", "3" becomes "03" and "1/" becomes
// "01/". A pasted 12/2030 or 2030-12 becomes 12/30, and text with other characters is left as typed.
export function formatExpiryInput(value: string): string {
  if (!/^[\d /.-]*$/.test(value)) {
    return value;
  }
  const yearFirst = value.match(/^\s*(\d{4})\s*[/.-]\s*(\d{1,2})\s*$/);
  if (yearFirst) {
    return `${yearFirst[2].padStart(2, "0")}/${yearFirst[1].slice(2)}`;
  }
  const separated = value.match(/^\s*(\d{1,2})\s*[/.-]\s*(\d*)\s*$/);
  if (separated) {
    const year = separated[2].length === 4 ? separated[2].slice(2) : separated[2].slice(0, 2);
    return `${separated[1].padStart(2, "0")}/${year}`;
  }
  let digits = value.replace(/\D/g, "");
  if (/^([2-9]|1[3-9])/.test(digits)) {
    digits = `0${digits}`;
  }
  if (digits.length <= 2) {
    return digits;
  }
  return `${digits.slice(0, 2)}/${digits.length === 6 ? digits.slice(4) : digits.slice(2, 4)}`;
}

// formatInputAsTyped puts format(input.value) into the input and keeps the caret after the same number of digits.
// Backspace over a separator removes the digit before it, so a space or slash the formatter adds never blocks deleting.
export function formatInputAsTyped(input: HTMLInputElement, format: (value: string) => string, previousValue: string, inputType: string): string {
  let value = input.value;
  let caret = input.selectionStart ?? value.length;
  const isCaretAtEnd = caret === value.length;
  if (inputType === "deleteContentBackward" && caret > 0 && format(value) === previousValue) {
    value = value.slice(0, caret - 1) + value.slice(caret);
    caret -= 1;
  }
  const formatted = format(value);
  if (formatted === input.value) {
    return formatted;
  }
  const digitsBeforeCaret = value.slice(0, caret).replace(/\D/g, "").length;
  let position = 0;
  for (let seen = 0; position < formatted.length && seen < digitsBeforeCaret; position++) {
    if (/\d/.test(formatted[position])) {
      seen++;
    }
  }
  input.value = formatted;
  if (isCaretAtEnd) {
    position = formatted.length;
  }
  input.setSelectionRange(position, position);
  return formatted;
}

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

// formatRoleWithArticle is a vault or organization role inside a sentence: "an owner", "an editor", "a reader", "an admin", "a member".
export function formatRoleWithArticle(role: string): string {
  return `${/^[aeiou]/.test(role) ? "an" : "a"} ${role}`;
}

// formatCount is a count with its noun: "1 member", "3 members".
export function formatCount(count: number, singular: string, plural = `${singular}s`): string {
  return `${count} ${count === 1 ? singular : plural}`;
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

// formatLastUse is "Last used just now" or "Last used 3 hours ago", for paired browsers and launcher apps.
export function formatLastUse(timestamp: string, now = new Date()): string {
  const lastSeen = formatLastSeen(timestamp, now);
  return lastSeen ? `Last used ${lastSeen === "Active now" ? "just now" : lastSeen.toLowerCase()}` : "";
}

// describeAgentSession names a coding agent session by its harness and the app it runs in: "omp in Tern", or "omp".
export function describeAgentSession(session: { harnessName: string; hostName: string }): string {
  return session.hostName ? `${session.harnessName} in ${session.hostName}` : session.harnessName;
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

// allIconPreset is core/item's IconPresets, in the picker's order; each preset is drawn with the AppIcon of its name.
export const allIconPreset = [
  "bank", "card", "wallet", "crypto", "cart", "gift", "mail", "chat", "social", "work", "home", "school",
  "cloud", "server", "database", "code", "terminal", "wifi", "globe", "game", "music", "video", "health",
  "travel", "car", "key", "lock", "shield", "star", "heart",
] as const satisfies readonly IconName[];

// allIconColor is core/item's IconColors; "" is the neutral tile.
export const allIconColor = ["red", "orange", "yellow", "green", "teal", "blue", "indigo", "purple", "pink", "gray"] as const;

// getPresetGlyph returns the icon of a preset, or undefined for one from a newer app, which draws the category icon.
export function getPresetGlyph(preset: string | undefined): IconName | undefined {
  return (allIconPreset as readonly string[]).includes(preset ?? "") ? (preset as IconName) : undefined;
}

// getWebsiteIconURL is the website whose icon a login shows when it has no icon of its own: its first https URL.
// Other item types keep their category icon.
export function getWebsiteIconURL(vaultItem: { category: string; urls?: string[]; icon?: unknown }): string {
  if ((vaultItem.category !== "" && vaultItem.category !== "login") || vaultItem.icon) {
    return "";
  }
  return (vaultItem.urls ?? []).find((link) => /^https:\/\//i.test(link.trim()))?.trim() ?? "";
}
