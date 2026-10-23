#!/usr/bin/env node
// Development stand-in for hisar-browser-host + the desktop app: speaks the native messaging protocol over stdio with data from a JSON state file.
import { createHash, createHmac, randomBytes, randomInt } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { endianness, homedir } from "node:os";
import { pathToFileURL } from "node:url";
import { isIP } from "node:net";
import { handlePasskeyRequest, PasskeyError } from "./fake-passkey-authenticator.mjs";

export const protocolVersion = 1;
const maximumMessageBytes = 1024 * 1024;
const pairingLifetimeMilliseconds = 2 * 60 * 1000;
const maximumPairingAttempt = 5;

export function createInitialState() {
  return {
    state: "unlocked",
    email: "alex@example.com",
    isOffline: false,
    isDesktopUnavailable: false,
    unlockDelayMilliseconds: 1500,
    unlockAt: null,
    pairingCode: null,
    pendingPairing: null,
    allPairing: [],
    exactHostOnly: false,
    // features: what the app announces in hello; an older app announces none.
    features: ["addresses", "vault", "unlock-password", "cards", "identities", "save-options", "create-items", "save-confirmation"],
    allVault: [
      { vaultID: "vault-personal", name: "Personal", isPersonal: true },
      { vaultID: "vault-family", name: "Family", isPersonal: false },
    ],
    allIdentity: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-alex",
        title: "Alex",
        summary: "Alex Example · alex@example.com",
        firstName: "Alex",
        middleName: "",
        lastName: "Example",
        email: "alex@example.com",
        phone: "+1 555 0100",
        birthDate: "1990-04-02",
        address: { street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "62704", country: "US" },
      },
    ],
    allCard: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-visa",
        title: "Visa",
        brand: "Visa",
        last4: "1111",
        expiry: "12/30",
        cardholder: "Alex Example",
        number: "4111111111111111",
        expiryMonth: "12",
        expiryYear: "2030",
        securityCode: "123",
      },
    ],
    // masterPassword is what unlock.password accepts.
    masterPassword: "master password",
    allAddress: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-home",
        title: "Home",
        source: "address",
        street: "1 Main St\nApt 4",
        city: "Springfield",
        region: "IL",
        postalCode: "62704",
        country: "US",
      },
    ],
    allItem: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-visa",
        category: "credit-card",
        title: "Visa",
        subtitle: "Visa •••• 1111",
        fields: [
          { id: "field:0", label: "Cardholder", kind: "text", value: "Alex Example" },
          { id: "field:1", label: "Card number", kind: "secret", value: "4111 1111 1111 1111" },
          { id: "field:2", label: "Expiry", kind: "text", value: "12/2030" },
          { id: "field:3", label: "Security code", kind: "secret", value: "123" },
        ],
      },
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-wifi",
        category: "secure-note",
        title: "Wi-Fi",
        subtitle: "",
        fields: [{ id: "notes", label: "Notes", kind: "notes", value: "Network: Example" }],
      },
    ],
    allFavoriteItemID: [],
    allLogin: [
      {
        vaultID: "vault-personal",
        vaultName: "Personal",
        itemID: "item-example",
        title: "Example",
        username: "alex@example.com",
        password: "correct horse battery staple",
        url: "https://example.com",
      },
    ],
  };
}

export function readState(stateFile) {
  if (!existsSync(stateFile)) return createInitialState();
  return { ...createInitialState(), ...JSON.parse(readFileSync(stateFile, "utf8")) };
}

export function writeState(stateFile, state) {
  mkdirSync(dirname(stateFile), { recursive: true });
  const temporaryFile = `${stateFile}.${process.pid}.tmp`;
  writeFileSync(temporaryFile, JSON.stringify(state, null, 2), { mode: 0o600 });
  renameSync(temporaryFile, stateFile);
}

function hashToken(token) {
  return createHash("sha256").update(token).digest("hex");
}

class ProtocolError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

function parseItemUrl(url) {
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(url) ? url : `https://${url}`;
  return new URL(withScheme);
}

function getPort(url) {
  if (url.port) return url.port;
  return url.protocol === "http:" ? "80" : url.protocol === "https:" ? "443" : "";
}

function getRegistrableDomain(host) {
  const allLabel = host.split(".");
  return allLabel.slice(-2).join(".");
}

export function doesUrlMatch(itemUrl, pageUrl, exactHostOnly) {
  let item;
  let page;
  try {
    item = parseItemUrl(itemUrl);
    page = new URL(pageUrl);
  } catch {
    return false;
  }
  const isSchemeAccepted = item.protocol === page.protocol || (item.protocol === "http:" && page.protocol === "https:");
  if (!isSchemeAccepted) return false;
  const isDefaultPortPair = !item.port && !page.port;
  if (!isDefaultPortPair && getPort(item) !== getPort(page)) return false;
  const itemHost = item.hostname.replace(/^\[|\]$/g, "");
  const pageHost = page.hostname.replace(/^\[|\]$/g, "");
  if (exactHostOnly || isIP(itemHost) || isIP(pageHost) || itemHost === "localhost" || pageHost === "localhost") {
    return itemHost === pageHost;
  }
  return getRegistrableDomain(itemHost) === getRegistrableDomain(pageHost);
}

function decodeBase32(text) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const character of text.toUpperCase().replace(/[\s=-]/g, "")) {
    const value = alphabet.indexOf(character);
    if (value < 0) throw new ProtocolError("not-found", "The one-time password is not valid");
    bits += value.toString(2).padStart(5, "0");
  }
  const allByte = [];
  for (let index = 0; index + 8 <= bits.length; index += 8) allByte.push(Number.parseInt(bits.slice(index, index + 8), 2));
  return Buffer.from(allByte);
}

// generateOneTimeCode: RFC 6238 with SHA-1, 6 digits and 30 s, for a base32 setup key.
export function generateOneTimeCode(setupKey, now) {
  const period = 30;
  const counter = Math.floor(now / 1000 / period);
  const counterBytes = Buffer.alloc(8);
  counterBytes.writeBigUInt64BE(BigInt(counter));
  const digest = createHmac("sha1", decodeBase32(setupKey)).update(counterBytes).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const value = digest.readUInt32BE(offset) & 0x7fffffff;
  return { code: String(value % 1_000_000).padStart(6, "0"), expiresAt: new Date((counter + 1) * period * 1000).toISOString().replace(".000Z", "Z"), period };
}

function generatePassword({ length = 24, digits = true, symbols = true } = {}) {
  const allLetter = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ";
  const allDigit = "23456789";
  const allSymbol = "!#$%&*+-=?@^_~";
  const alphabet = allLetter + (digits ? allDigit : "") + (symbols ? allSymbol : "");
  const boundedLength = Math.min(Math.max(Number(length) || 24, 8), 128);
  let password = "";
  for (let index = 0; index < boundedLength; index += 1) password += alphabet[randomInt(alphabet.length)];
  return password;
}

function requireString(request, name) {
  const value = request[name];
  if (typeof value !== "string" || value === "") throw new ProtocolError("invalid-request", `${name} is required`);
  return value;
}

function applyPendingUnlock(state, now) {
  if (state.state === "locked" && state.unlockAt !== null && state.unlockAt <= now) {
    state.state = "unlocked";
    state.unlockAt = null;
  }
}

function requireUnlocked(state) {
  if (state.state === "signed-out") throw new ProtocolError("signed-out", "Hisar is signed out");
  if (state.state === "locked") throw new ProtocolError("locked", "Hisar is locked");
}

// listAllItem is every item as items.list and items.get see it: the logins, then the other items.
function listAllItem(state) {
  const allLoginItem = state.allLogin.map((login) => ({
    vaultID: login.vaultID,
    vaultName: login.vaultName,
    itemID: login.itemID,
    category: "login",
    title: login.title,
    subtitle: login.username,
    url: login.url,
    hasOneTimePassword: Boolean(login.oneTimePassword),
    fields: [
      ...(login.username ? [{ id: "username", label: "Username", kind: "text", value: login.username }] : []),
      { id: "password", label: "Password", kind: "secret", value: login.password },
      ...(login.oneTimePassword ? [{ id: "one-time-code", label: "One-time code", kind: "one-time-code" }] : []),
      { id: "url:0", label: "Website", kind: "url", value: login.url },
    ],
    oneTimePassword: login.oneTimePassword,
  }));
  return [...allLoginItem, ...(state.allItem ?? []).map((item) => ({ hasOneTimePassword: false, ...item }))];
}

function summarizeItem(state, item) {
  const { vaultID, vaultName, itemID, category, title, subtitle, url, hasOneTimePassword } = item;
  return { vaultID, vaultName, itemID, category, title, subtitle, ...(url ? { url } : {}), hasOneTimePassword, isFavorite: (state.allFavoriteItemID ?? []).includes(itemID) };
}

function requireVaultItem(state, request) {
  if (!state.features?.includes("vault")) throw new ProtocolError("invalid-request", `Unknown type ${String(request.type)}`);
  requireUnlocked(state);
  const item = listAllItem(state).find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
  if (!item) throw new ProtocolError("not-found", "No such item");
  return item;
}

function findAllMatchingLogin(state, url) {
  return state.allLogin.filter((login) => doesUrlMatch(login.url, url, state.exactHostOnly));
}

export function handleRequest(state, request, now = Date.now(), log = () => undefined) {
  if (state.isDesktopUnavailable) throw new ProtocolError("desktop-unavailable", "Hisar is not running");
  applyPendingUnlock(state, now);
  const type = request.type;
  if (type !== "hello" && type !== "pair.start" && type !== "pair.finish") {
    const token = typeof request.token === "string" ? request.token : "";
    if (!token || !state.allPairing.some((pairing) => pairing.tokenHash === hashToken(token))) {
      throw new ProtocolError("not-paired", "This browser is not paired with Hisar");
    }
    if (state.updateRequired?.target === "app") {
      throw new ProtocolError("update-required", `Hisar needs an update: the server requires version ${state.updateRequired.minimumVersion} or later. Open Hisar to update.`);
    }
  }
  switch (type) {
    case "hello": {
      const instanceID = requireString(request, "instanceID");
      return {
        protocolVersion,
        isPaired: state.allPairing.some((pairing) => pairing.instanceID === instanceID),
        state: state.state,
        ...(state.updateRequired ? { updateRequired: state.updateRequired } : {}),
        ...(state.features?.length ? { features: state.features } : {}),
      };
    }
    case "pair.start": {
      const instanceID = requireString(request, "instanceID");
      const browser = requireString(request, "browser");
      if (state.state === "signed-out") throw new ProtocolError("signed-out", "Sign in to Hisar first");
      requireUnlocked(state);
      const code = state.pairingCode ?? String(randomInt(1000000)).padStart(6, "0");
      state.pendingPairing = { instanceID, browser, code, expiresAt: now + pairingLifetimeMilliseconds, attemptCount: 0 };
      log(`Pair ${browser}? Code ${code}`);
      return {};
    }
    case "pair.finish": {
      const instanceID = requireString(request, "instanceID");
      const code = requireString(request, "code");
      requireUnlocked(state);
      const pending = state.pendingPairing;
      if (!pending || pending.instanceID !== instanceID || pending.expiresAt < now) {
        state.pendingPairing = null;
        throw new ProtocolError("pairing-failed", "The pairing code expired. Start pairing again");
      }
      if (pending.code !== code) {
        pending.attemptCount += 1;
        if (pending.attemptCount >= maximumPairingAttempt) state.pendingPairing = null;
        throw new ProtocolError("pairing-failed", "The code does not match");
      }
      const token = randomBytes(32).toString("hex");
      state.allPairing = state.allPairing.filter((pairing) => pairing.instanceID !== instanceID);
      state.allPairing.push({ instanceID, browser: pending.browser, tokenHash: hashToken(token), pairedAt: now });
      state.pendingPairing = null;
      return { token };
    }
    case "status":
      return { state: state.state, email: state.state === "signed-out" ? "" : state.email, isOffline: state.isOffline };
    case "unlock":
      if (state.state === "signed-out") throw new ProtocolError("signed-out", "Hisar is signed out");
      if (state.state === "locked") state.unlockAt = now + state.unlockDelayMilliseconds;
      return {};
    case "unlock.password": {
      if (!state.features?.includes("unlock-password")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      if (state.state === "signed-out") throw new ProtocolError("signed-out", "Hisar is signed out");
      if (request.password !== state.masterPassword) throw new ProtocolError("wrong-password", "That master password is not right");
      state.state = "unlocked";
      state.unlockAt = null;
      return { state: state.state, email: state.email, isOffline: state.isOffline };
    }
    case "logins.match": {
      requireUnlocked(state);
      const url = requireString(request, "url");
      const allLogin = findAllMatchingLogin(state, url)
        .map(({ vaultID, itemID, title, username, vaultName, oneTimePassword }) => ({
          vaultID,
          itemID,
          title,
          username,
          vaultName,
          hasOneTimePassword: Boolean(oneTimePassword),
        }))
        .sort((first, second) => first.title.localeCompare(second.title));
      return { logins: allLogin };
    }
    case "logins.fill": {
      requireUnlocked(state);
      const url = requireString(request, "url");
      const login = state.allLogin.find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
      if (!login) throw new ProtocolError("not-found", "No such login");
      if (!doesUrlMatch(login.url, url, state.exactHostOnly)) throw new ProtocolError("no-match", "This login is not for this page");
      return { username: login.username, password: login.password, hasOneTimePassword: Boolean(login.oneTimePassword) };
    }
    case "logins.code": {
      requireUnlocked(state);
      const url = requireString(request, "url");
      const login = state.allLogin.find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
      if (!login || !doesUrlMatch(login.url, url, state.exactHostOnly)) throw new ProtocolError("no-match", "This login is not for this page");
      if (!login.oneTimePassword) throw new ProtocolError("not-found", "This login has no one-time password");
      state.allCodeRequest = [...(state.allCodeRequest ?? []), { itemID: login.itemID, url }];
      return generateOneTimeCode(login.oneTimePassword, now);
    }
    case "logins.prepare": {
      requireUnlocked(state);
      if (typeof request.tabID !== "string" || !request.tabID) throw new ProtocolError("update-required", "update the browser extension to save logins");
      const url = requireString(request, "url");
      const username = typeof request.username === "string" ? request.username : "";
      const password = requireString(request, "password");
      const login = findAllMatchingLogin(state, url).find((candidate) => candidate.username.toLowerCase() === username.toLowerCase());
      state.pendingSaves ??= {};
      for (const [key, pending] of Object.entries(state.pendingSaves)) {
        if (pending.expiresAt < now || (pending.token === request.token && pending.tabID === request.tabID)) delete state.pendingSaves[key];
      }
      if (login?.password === password) return { status: "unchanged", vaultID: login.vaultID, itemID: login.itemID, title: login.title };
      if (Object.keys(state.pendingSaves).length >= 128) throw new ProtocolError("invalid-request", "too many pending saves; try again after two minutes");
      const saveContext = randomBytes(20).toString("hex");
      state.pendingSaves[saveContext] = { token: request.token, tabID: request.tabID, url, password, username, itemID: login?.itemID, vaultID: login?.vaultID, expiresAt: now + pairingLifetimeMilliseconds, email: state.email };
      if (!login) return { status: "new", saveContext };
      return { status: "changed", vaultID: login.vaultID, itemID: login.itemID, title: login.title, saveContext };
    }
    case "logins.review": {
      requireUnlocked(state);
      const pending = state.pendingSaves?.[request.saveContext];
      if (!pending || pending.token !== request.token || pending.email !== state.email || pending.expiresAt < now) throw new ProtocolError("not-found", "This save confirmation expired; submit the login again");
      return {};
    }
    case "logins.save": {
      requireUnlocked(state);
      if (state.isOffline) throw new ProtocolError("offline", "Hisar is offline");
      const url = requireString(request, "url");
      const username = typeof request.username === "string" ? request.username : "";
      const password = requireString(request, "password");
      const pending = state.pendingSaves?.[request.saveContext];
      if (!pending || pending.token !== request.token || pending.url !== url || pending.password !== password || pending.itemID !== request.itemID || pending.email !== state.email || pending.expiresAt < now) throw new ProtocolError("invalid-request", "This save confirmation expired; submit the login again");
      delete state.pendingSaves[request.saveContext];
      if (request.itemID) {
        const login = state.allLogin.find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
        if (!login) throw new ProtocolError("not-found", "No such login");
        login.username = username;
        login.password = password;
        return { vaultID: login.vaultID, itemID: login.itemID };
      }
      const host = new URL(url).host;
      const vault = (state.allVault ?? []).find((candidate) => candidate.vaultID === (request.vaultID || "vault-personal")) ?? { vaultID: "vault-personal", name: "Personal" };
      if (request.vaultID && !(state.allVault ?? []).some((candidate) => candidate.vaultID === request.vaultID)) {
        throw new ProtocolError("invalid-request", "You cannot add items to that vault");
      }
      const login = {
        vaultID: vault.vaultID,
        vaultName: vault.name,
        itemID: `item-${randomBytes(6).toString("hex")}`,
        title: typeof request.title === "string" && request.title.trim() ? request.title.trim() : host,
        username,
        password,
        url: new URL(url).origin,
      };
      state.allLogin.push(login);
      return { vaultID: login.vaultID, itemID: login.itemID };
    }
    case "addresses.list": {
      if (!state.features?.includes("addresses")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const allAddress = (state.allAddress ?? []).map(({ vaultID, itemID, title, source, vaultName, city, country }) => ({
        vaultID,
        itemID,
        title,
        summary: [city, country].filter(Boolean).join(", "),
        source,
        vaultName,
      }));
      return { addresses: allAddress };
    }
    case "addresses.fill": {
      if (!state.features?.includes("addresses")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const address = (state.allAddress ?? []).find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
      if (!address) throw new ProtocolError("not-found", "No such address");
      state.allAddressRequest = [...(state.allAddressRequest ?? []), address.itemID];
      const { street, city, region, postalCode, country } = address;
      return { street, city, region, postalCode, country };
    }
    case "identities.list": {
      if (!state.features?.includes("identities")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      return { identities: (state.allIdentity ?? []).map(({ vaultID, vaultName, itemID, title, summary }) => ({ vaultID, vaultName, itemID, title, summary })) };
    }
    case "identities.fill": {
      if (!state.features?.includes("identities")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const identity = (state.allIdentity ?? []).find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
      if (!identity) throw new ProtocolError("not-found", "No such identity");
      state.allIdentityRequest = [...(state.allIdentityRequest ?? []), identity.itemID];
      const { firstName, middleName, lastName, email, phone, birthDate, address } = identity;
      return { firstName, middleName, lastName, email, phone, birthDate, ...(address ? { address } : {}) };
    }
    case "cards.list": {
      if (!state.features?.includes("cards")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const allCard = (state.allCard ?? []).map(({ vaultID, vaultName, itemID, title, brand, last4, expiry }) => ({ vaultID, vaultName, itemID, title, brand, last4, expiry }));
      return { cards: allCard };
    }
    case "cards.fill": {
      if (!state.features?.includes("cards")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const card = (state.allCard ?? []).find((candidate) => candidate.vaultID === request.vaultID && candidate.itemID === request.itemID);
      if (!card) throw new ProtocolError("not-found", "No such card");
      state.allCardRequest = [...(state.allCardRequest ?? []), card.itemID];
      const { cardholder, number, expiryMonth, expiryYear, securityCode, brand } = card;
      return { cardholder, number, expiryMonth, expiryYear, securityCode, brand };
    }
    case "vaults.list":
      if (!state.features?.includes("save-options") && !state.features?.includes("vault")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      return { vaults: state.allVault ?? [] };
    case "items.create": {
      if (!state.features?.includes("create-items")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const vault = (state.allVault ?? []).find((candidate) => candidate.vaultID === request.vaultID);
      if (!vault) throw new ProtocolError("invalid-request", "You cannot add items to that vault");
      const item = /** @type {Record<string, string>} */ (request.item ?? {});
      if (!item.title?.trim()) throw new ProtocolError("invalid-request", "enter a title");
      const itemID = `item-${randomBytes(6).toString("hex")}`;
      if (item.category === "login") {
        state.allLogin.push({ vaultID: vault.vaultID, vaultName: vault.name, itemID, title: item.title, username: item.username ?? "", password: item.password ?? "", url: item.url ?? "" });
      } else {
        state.allItem = [...(state.allItem ?? []), { vaultID: vault.vaultID, vaultName: vault.name, itemID, category: item.category, title: item.title, subtitle: "", fields: [] }];
      }
      return summarizeItem(state, listAllItem(state).find((candidate) => candidate.itemID === itemID));
    }
    case "items.list": {
      if (!state.features?.includes("vault")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      requireUnlocked(state);
      const query = typeof request.query === "string" ? request.query.toLowerCase() : "";
      const allItem = listAllItem(state)
        .filter((item) => !request.category || item.category === request.category)
        .filter((item) => !request.isFavoriteOnly || (state.allFavoriteItemID ?? []).includes(item.itemID))
        .filter((item) => !query || [item.title, item.subtitle, item.vaultName, item.url ?? ""].some((text) => text.toLowerCase().includes(query)))
        .sort((first, second) => first.title.localeCompare(second.title));
      return { items: allItem.map((item) => summarizeItem(state, item)) };
    }
    case "items.get": {
      const item = requireVaultItem(state, request);
      const fields = item.fields.map((field) => (field.kind === "secret" || field.kind === "one-time-code" ? { id: field.id, label: field.label, kind: field.kind } : field));
      return { ...summarizeItem(state, item), fields };
    }
    case "items.reveal": {
      const item = requireVaultItem(state, request);
      const field = item.fields.find((candidate) => candidate.id === request.fieldID);
      if (!field) throw new ProtocolError("not-found", "No such field");
      state.allRevealRequest = [...(state.allRevealRequest ?? []), `${item.itemID}/${field.id}`];
      if (field.kind === "one-time-code") return { value: generateOneTimeCode(item.oneTimePassword, now).code };
      return { value: field.value ?? "" };
    }
    case "items.code": {
      const item = requireVaultItem(state, request);
      if (!item.oneTimePassword) throw new ProtocolError("not-found", "This item has no one-time password");
      return generateOneTimeCode(item.oneTimePassword, now);
    }
    case "items.icon":
      if (!state.features?.includes("vault")) throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
      return {};
    case "items.favorite": {
      const item = requireVaultItem(state, request);
      const allOther = (state.allFavoriteItemID ?? []).filter((itemID) => itemID !== item.itemID);
      state.allFavoriteItemID = request.isFavorite === true ? [...allOther, item.itemID] : allOther;
      return {};
    }
    case "password.generate":
      if (state.state === "signed-out") throw new ProtocolError("signed-out", "Hisar is signed out");
      return { password: generatePassword(request) };
    default:
      if (typeof type === "string" && type.startsWith("passkeys.")) return handlePasskeyRequest(state, request, now, findAllMatchingLogin);
      throw new ProtocolError("invalid-request", `Unknown type ${String(type)}`);
  }
}

export function respond(state, request, now, log) {
  const id = typeof request?.id === "string" ? request.id : "";
  try {
    if (!id || typeof request.type !== "string") throw new ProtocolError("invalid-request", "id and type are required");
    return { id, ok: true, result: handleRequest(state, request, now, log) };
  } catch (error) {
    const code = error instanceof ProtocolError || error instanceof PasskeyError ? error.code : "internal";
    return { id, ok: false, error: { code, message: error instanceof Error ? error.message : String(error) } };
  }
}

export function encodeFrame(message) {
  const body = Buffer.from(JSON.stringify(message), "utf8");
  const header = Buffer.alloc(4);
  if (endianness() === "LE") header.writeUInt32LE(body.length);
  else header.writeUInt32BE(body.length);
  return Buffer.concat([header, body]);
}

export function createFrameReader(onMessage) {
  let buffered = Buffer.alloc(0);
  return (chunk) => {
    buffered = Buffer.concat([buffered, chunk]);
    while (buffered.length >= 4) {
      const length = endianness() === "LE" ? buffered.readUInt32LE(0) : buffered.readUInt32BE(0);
      if (length > maximumMessageBytes) throw new Error(`Message of ${length} bytes is too large`);
      if (buffered.length < 4 + length) return;
      const body = buffered.subarray(4, 4 + length).toString("utf8");
      buffered = buffered.subarray(4 + length);
      onMessage(JSON.parse(body));
    }
  };
}

function runHost() {
  const stateFile = process.env.HISAR_FAKE_HOST_STATE || `${homedir()}/.hisar-fake-host/state.json`;
  const log = (line) => process.stderr.write(`hisar-fake-host: ${line}\n`);
  log(`started for ${process.argv[2] ?? "unknown origin"}, state ${stateFile}`);
  const readFrame = createFrameReader((request) => {
    const state = readState(stateFile);
    const response = respond(state, request, Date.now(), log);
    writeState(stateFile, state);
    process.stdout.write(encodeFrame(response));
  });
  process.stdin.on("data", (chunk) => {
    try {
      readFrame(chunk);
    } catch (error) {
      log(String(error));
      process.exit(1);
    }
  });
  process.stdin.on("end", () => process.exit(0));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) runHost();
