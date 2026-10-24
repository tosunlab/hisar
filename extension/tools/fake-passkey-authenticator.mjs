// Development stand-in for the desktop's passkey authenticator, used by the fake host.
import { createHash, createHmac, createPrivateKey, generateKeyPairSync, hkdfSync, randomBytes, sign } from "node:crypto";

export const hisarAAGUID = "fb041679-042a-49ae-b616-8779e2ac14ce";
const prfSecretInfo = "hisar passkey prf v1";
const maximumPRFInputBytes = 1024;
const allSignalType = ["passkeys.signalUnknownCredential", "passkeys.signalAllAcceptedCredentials", "passkeys.signalCurrentUserDetails"];

class PasskeyError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
  }
}

function encodeBase64URL(buffer) {
  return Buffer.from(buffer).toString("base64url");
}

function decodeBase64URL(text, name) {
  if (typeof text !== "string" || !/^[A-Za-z0-9_-]+$/.test(text)) throw new PasskeyError("invalid-request", `${name} is not base64url`);
  return Buffer.from(text, "base64url");
}

function encodeHeader(majorType, value) {
  if (value < 24) return Buffer.from([(majorType << 5) | value]);
  if (value < 256) return Buffer.from([(majorType << 5) | 24, value]);
  const header = Buffer.alloc(3);
  header[0] = (majorType << 5) | 25;
  header.writeUInt16BE(value, 1);
  return header;
}

function encodeInteger(value) {
  return value >= 0 ? encodeHeader(0, value) : encodeHeader(1, -1 - value);
}

function encodeBytes(bytes) {
  return Buffer.concat([encodeHeader(2, bytes.length), bytes]);
}

function encodeText(text) {
  const bytes = Buffer.from(text, "utf8");
  return Buffer.concat([encodeHeader(3, bytes.length), bytes]);
}

export function resolveRelyingPartyID(origin, requestedID) {
  let parsed;
  try {
    parsed = new URL(origin);
  } catch {
    throw new PasskeyError("invalid-request", "invalid origin");
  }
  const host = parsed.hostname;
  const isLocalhost = host === "localhost" || host.endsWith(".localhost");
  if (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && isLocalhost)) throw new PasskeyError("invalid-request", "invalid origin");
  const relyingPartyID = (requestedID ?? "").toLowerCase();
  if (!relyingPartyID || relyingPartyID === host) return host;
  if (!host.endsWith(`.${relyingPartyID}`) || !relyingPartyID.includes(".")) throw new PasskeyError("invalid-request", "the RP ID does not belong to the origin");
  return relyingPartyID;
}

function buildClientDataJSON(type, passkey) {
  const clientData = { type, challenge: passkey.challenge, origin: passkey.origin, crossOrigin: passkey.isCrossOrigin === true };
  if (passkey.isCrossOrigin) clientData.topOrigin = passkey.topOrigin;
  return Buffer.from(JSON.stringify(clientData), "utf8");
}

function buildAuthenticatorData(relyingPartyID, flags, attestedCredentialData = Buffer.alloc(0)) {
  return Buffer.concat([createHash("sha256").update(relyingPartyID).digest(), Buffer.from([flags]), Buffer.alloc(4), attestedCredentialData]);
}

// The PRF secret derivation, as the desktop computes it.
export function derivePRFSecret(privateKeyDERBase64) {
  const key = createPrivateKey({ key: Buffer.from(privateKeyDERBase64, "base64"), format: "der", type: "pkcs8" });
  const scalar = Buffer.from(key.export({ format: "jwk" }).d, "base64url");
  return Buffer.from(hkdfSync("sha256", scalar, Buffer.alloc(0), prfSecretInfo, 32));
}

export function evaluatePRF(secret, input) {
  const transformed = createHash("sha256").update(Buffer.concat([Buffer.from("WebAuthn PRF", "utf8"), Buffer.from([0]), input])).digest();
  return createHmac("sha256", secret).update(transformed).digest();
}

function decodePRFInput(text) {
  if (typeof text !== "string" || !/^[A-Za-z0-9_-]*$/.test(text)) throw new PasskeyError("invalid-request", "PRF input is not base64url");
  const input = Buffer.from(text, "base64url");
  if (input.length > maximumPRFInputBytes) throw new PasskeyError("invalid-request", "PRF input is too long");
  return input;
}

function evaluatePRFValues(values, privateKeyDERBase64) {
  const secret = derivePRFSecret(privateKeyDERBase64);
  const results = { first: encodeBase64URL(evaluatePRF(secret, decodePRFInput(values.first))) };
  if (values.second !== undefined) results.second = encodeBase64URL(evaluatePRF(secret, decodePRFInput(values.second)));
  return results;
}

function selectPRFValues(prf, allAllowed, credentialID) {
  const allEntry = Object.entries(prf.evalByCredential ?? {});
  if (allEntry.length > 0 && allAllowed.length === 0) throw new PasskeyError("invalid-request", "evalByCredential needs allowCredentials");
  if (allEntry.some(([key]) => !allAllowed.includes(key))) throw new PasskeyError("invalid-request", "evalByCredential key outside allowCredentials");
  return allEntry.find(([key]) => key === credentialID)?.[1] ?? prf.eval;
}

function handleSignal(state, type, passkey) {
  if (typeof passkey.relyingPartyId !== "string" || passkey.relyingPartyId === "") throw new PasskeyError("invalid-request", "relyingPartyId is required");
  const relyingPartyID = resolveRelyingPartyID(passkey.origin, passkey.relyingPartyId);
  if (state.state !== "unlocked") return { changedPasskeyCount: 0 };
  state.allPasskey ??= [];
  let changedPasskeyCount = 0;
  const allKept = [];
  for (const stored of state.allPasskey) {
    if (stored.relyingPartyId !== relyingPartyID) {
      allKept.push(stored);
      continue;
    }
    const isUser = stored.userHandle === passkey.userHandle;
    if (type === "passkeys.signalUnknownCredential" && stored.credentialId === passkey.credentialId) {
      changedPasskeyCount++;
      continue;
    }
    if (type === "passkeys.signalAllAcceptedCredentials" && isUser && !(passkey.allAcceptedCredentialIds ?? []).includes(stored.credentialId)) {
      changedPasskeyCount++;
      continue;
    }
    if (type === "passkeys.signalCurrentUserDetails" && isUser && (stored.userName !== passkey.userName || stored.userDisplayName !== passkey.userDisplayName)) {
      changedPasskeyCount++;
      allKept.push({ ...stored, userName: passkey.userName ?? "", userDisplayName: passkey.userDisplayName ?? "" });
      continue;
    }
    allKept.push(stored);
  }
  state.allPasskey = allKept;
  return { changedPasskeyCount };
}

function requireUnlocked(state) {
  if (state.state === "signed-out") throw new PasskeyError("signed-out", "Hisar is signed out");
  if (state.state === "locked") throw new PasskeyError("locked", "Hisar is locked");
}

function findAllPasskey(state, relyingPartyID) {
  state.allPasskey ??= [];
  return state.allPasskey.filter((passkey) => passkey.relyingPartyId === relyingPartyID);
}

function findLogin(state, vaultID, itemID) {
  return state.allLogin.find((login) => login.vaultID === vaultID && login.itemID === itemID);
}

export function handlePasskeyRequest(state, request, now, findAllMatchingLogin) {
  const passkey = request.passkey;
  if (request.type === "passkeys.cancel") return null;
  if (allSignalType.includes(request.type)) {
    if (typeof passkey !== "object" || passkey === null) throw new PasskeyError("invalid-request", "passkey parameters are required");
    return handleSignal(state, request.type, passkey);
  }
  requireUnlocked(state);
  if (typeof passkey !== "object" || passkey === null) throw new PasskeyError("invalid-request", "passkey parameters are required");
  const relyingPartyID = resolveRelyingPartyID(passkey.origin, passkey.relyingPartyId);
  const allMatch = findAllPasskey(state, relyingPartyID);
  const allAllowed = passkey.allowCredentials ?? [];
  const allExcluded = passkey.excludeCredentials ?? [];
  const needsVerification = passkey.userVerification !== "discouraged" || !!passkey.prf?.eval || Object.keys(passkey.prf?.evalByCredential ?? {}).length > 0;
  if ((request.type === "passkeys.create" || request.type === "passkeys.get") && needsVerification && state.isPasskeyVerificationDenied) {
    throw new PasskeyError("invalid-request", "Passkey verification was cancelled.");
  }
  switch (request.type) {
    case "passkeys.list":
      return {
        passkeys: allMatch
          .filter((match) => allAllowed.length === 0 || allAllowed.includes(match.credentialId))
          .map((match) => ({
            credentialId: match.credentialId,
            vaultID: match.vaultID,
            itemID: match.itemID,
            title: findLogin(state, match.vaultID, match.itemID)?.title ?? relyingPartyID,
            userName: match.userName,
            userDisplayName: match.userDisplayName,
          })),
      };
    case "passkeys.prepare":
      return {
        isExcluded: allMatch.some((match) => allExcluded.includes(match.credentialId)),
        logins: findAllMatchingLogin(state, passkey.origin).map(({ vaultID, itemID, title, username, vaultName }) => ({ vaultID, itemID, title, username, vaultName, isPersonal: state.allVault?.some((vault) => vault.vaultID === vaultID && vault.isPersonal) === true })),
      };
    case "passkeys.create": {
      if (allMatch.some((match) => allExcluded.includes(match.credentialId))) throw new PasskeyError("invalid-request", "Hisar already has a passkey for this account");
      const userHandle = encodeBase64URL(decodeBase64URL(passkey.userHandle, "userHandle"));
      decodeBase64URL(passkey.challenge, "challenge");
      let login;
      if (passkey.vaultID || passkey.itemID) {
        login = findAllMatchingLogin(state, passkey.origin).find((candidate) => candidate.vaultID === passkey.vaultID && candidate.itemID === passkey.itemID);
        if (!login) throw new PasskeyError("no-match", "This login is not saved for this site");
      } else {
        const host = new URL(passkey.origin).hostname;
        login = {
          vaultID: "vault-personal",
          vaultName: "Personal",
          itemID: `item-${randomBytes(6).toString("hex")}`,
          title: relyingPartyID.replace(/^www\./, ""),
          username: passkey.userName ?? "",
          password: "",
          url: host === relyingPartyID ? passkey.origin : `https://${relyingPartyID}`,
        };
        state.allLogin.push(login);
      }
      if (passkey.prf?.evalByCredential !== undefined) throw new PasskeyError("invalid-request", "evalByCredential is not supported on create");
      const { privateKey, publicKey } = generateKeyPairSync("ec", { namedCurve: "prime256v1" });
      const privateKeyDERBase64 = privateKey.export({ format: "der", type: "pkcs8" }).toString("base64");
      const prf = passkey.prf ? { enabled: true, ...(passkey.prf.eval ? { results: evaluatePRFValues(passkey.prf.eval, privateKeyDERBase64) } : {}) } : undefined;
      const jwk = publicKey.export({ format: "jwk" });
      const coseKey = Buffer.concat([
        encodeHeader(5, 5),
        encodeInteger(1),
        encodeInteger(2),
        encodeInteger(3),
        encodeInteger(-7),
        encodeInteger(-1),
        encodeInteger(1),
        encodeInteger(-2),
        encodeBytes(Buffer.from(jwk.x, "base64url")),
        encodeInteger(-3),
        encodeBytes(Buffer.from(jwk.y, "base64url")),
      ]);
      const credentialID = randomBytes(32);
      const credentialLength = Buffer.alloc(2);
      credentialLength.writeUInt16BE(credentialID.length);
      const attestedCredentialData = Buffer.concat([Buffer.from(hisarAAGUID.replaceAll("-", ""), "hex"), credentialLength, credentialID, coseKey]);
      const authenticatorData = buildAuthenticatorData(relyingPartyID, 0x59 | (needsVerification ? 0x04 : 0), attestedCredentialData);
      const attestationObject = Buffer.concat([
        encodeHeader(5, 3),
        encodeText("fmt"),
        encodeText("none"),
        encodeText("attStmt"),
        encodeHeader(5, 0),
        encodeText("authData"),
        encodeBytes(authenticatorData),
      ]);
      state.allPasskey = state.allPasskey.filter(
        (stored) => !(stored.vaultID === login.vaultID && stored.itemID === login.itemID && stored.relyingPartyId === relyingPartyID && stored.userHandle === userHandle),
      );
      state.allPasskey.push({
        credentialId: encodeBase64URL(credentialID),
        relyingPartyId: relyingPartyID,
        userHandle,
        userName: passkey.userName ?? "",
        userDisplayName: passkey.userDisplayName ?? "",
        privateKey: privateKeyDERBase64,
        vaultID: login.vaultID,
        itemID: login.itemID,
        createdAt: now,
      });
      return {
        credentialId: encodeBase64URL(credentialID),
        clientDataJSON: encodeBase64URL(buildClientDataJSON("webauthn.create", passkey)),
        attestationObject: encodeBase64URL(attestationObject),
        authenticatorData: encodeBase64URL(authenticatorData),
        publicKey: encodeBase64URL(publicKey.export({ format: "der", type: "spki" })),
        publicKeyAlgorithm: -7,
        vaultID: login.vaultID,
        itemID: login.itemID,
        ...(prf ? { prf } : {}),
      };
    }
    case "passkeys.get": {
      if (allAllowed.length > 0 && !allAllowed.includes(passkey.credentialId)) throw new PasskeyError("not-found", "passkey not found");
      const stored = allMatch.find((match) => match.credentialId === passkey.credentialId);
      if (!stored) throw new PasskeyError("not-found", "passkey not found");
      decodeBase64URL(passkey.challenge, "challenge");
      const selectedPRF = passkey.prf ? selectPRFValues(passkey.prf, allAllowed, stored.credentialId) : undefined;
      const prf = passkey.prf ? (selectedPRF ? { results: evaluatePRFValues(selectedPRF, stored.privateKey) } : {}) : undefined;
      const clientDataJSON = buildClientDataJSON("webauthn.get", passkey);
      const authenticatorData = buildAuthenticatorData(relyingPartyID, 0x19 | (needsVerification ? 0x04 : 0));
      const signedData = Buffer.concat([authenticatorData, createHash("sha256").update(clientDataJSON).digest()]);
      const key = createPrivateKey({ key: Buffer.from(stored.privateKey, "base64"), format: "der", type: "pkcs8" });
      return {
        credentialId: stored.credentialId,
        clientDataJSON: encodeBase64URL(clientDataJSON),
        authenticatorData: encodeBase64URL(authenticatorData),
        signature: encodeBase64URL(sign("sha256", signedData, { key, dsaEncoding: "der" })),
        userHandle: stored.userHandle,
        ...(prf ? { prf } : {}),
      };
    }
    default:
      throw new PasskeyError("invalid-request", `Unknown type ${String(request.type)}`);
  }
}

export { PasskeyError };
