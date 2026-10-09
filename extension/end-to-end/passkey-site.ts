import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import {
  generateAuthenticationOptions,
  generateRegistrationOptions,
  verifyAuthenticationResponse,
  verifyRegistrationResponse,
  type WebAuthnCredential,
} from "@simplewebauthn/server";

// A relying party on http://localhost (a secure context) that verifies with @simplewebauthn/server, independently of Hisar.
const page = `<!doctype html>
<html><head><meta charset="utf-8"><title>Passkey test site</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:300px} input,button{padding:8px;font-size:14px} pre{white-space:pre-wrap;max-width:560px}</style>
</head><body>
<h1>Passkey test site</h1>
<form onsubmit="return false">
  <input id="username" name="username" autocomplete="username webauthn" placeholder="Email">
  <button id="register" type="button">Create a passkey</button>
  <button id="sign-in" type="button">Sign in with a passkey</button>
  <button id="register-excluded" type="button">Create another passkey for this account</button>
  <button id="conditional" type="button">Start passkey autofill</button>
</form>
<pre id="status">ready</pre>
<script>
const toBytes = (text) => Uint8Array.from(atob(text.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (text.length % 4)) % 4)), (c) => c.charCodeAt(0));
const toText = (buffer) => btoa(String.fromCharCode(...new Uint8Array(buffer))).replace(/\\+/g, "-").replace(/\\//g, "_").replace(/=+$/, "");
const show = (value) => (document.getElementById("status").textContent = typeof value === "string" ? value : JSON.stringify(value));
async function post(path, body) {
  const response = await fetch(path, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body ?? {}) });
  return response.json();
}
async function register(isExcluded) {
  try {
    const options = await post("/registration/options", { isExcluded });
    const credential = await navigator.credentials.create({ publicKey: {
      ...options,
      challenge: toBytes(options.challenge),
      user: { ...options.user, id: toBytes(options.user.id) },
      excludeCredentials: (options.excludeCredentials ?? []).map((descriptor) => ({ ...descriptor, id: toBytes(descriptor.id) })),
    } });
    const json = {
      id: credential.id, rawId: toText(credential.rawId), type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment,
      clientExtensionResults: credential.getClientExtensionResults(),
      response: {
        clientDataJSON: toText(credential.response.clientDataJSON),
        attestationObject: toText(credential.response.attestationObject),
        transports: credential.response.getTransports(),
        publicKeyAlgorithm: credential.response.getPublicKeyAlgorithm(),
        publicKey: toText(credential.response.getPublicKey()),
        authenticatorData: toText(credential.response.getAuthenticatorData()),
      },
    };
    const result = await post("/registration/verify", json);
    show({ ...result, isPublicKeyCredential: credential instanceof PublicKeyCredential, isAttestationResponse: credential.response instanceof AuthenticatorAttestationResponse, toJSONMatches: JSON.stringify(credential.toJSON().response.attestationObject) === JSON.stringify(json.response.attestationObject) });
  } catch (error) {
    show({ error: error.name, message: error.message });
  }
}
async function signIn(mediation) {
  try {
    const options = await post("/authentication/options");
    const credential = await navigator.credentials.get({ mediation, publicKey: {
      ...options,
      challenge: toBytes(options.challenge),
      allowCredentials: (options.allowCredentials ?? []).map((descriptor) => ({ ...descriptor, id: toBytes(descriptor.id) })),
    } });
    const json = {
      id: credential.id, rawId: toText(credential.rawId), type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment,
      clientExtensionResults: credential.getClientExtensionResults(),
      response: {
        clientDataJSON: toText(credential.response.clientDataJSON),
        authenticatorData: toText(credential.response.authenticatorData),
        signature: toText(credential.response.signature),
        userHandle: credential.response.userHandle ? toText(credential.response.userHandle) : undefined,
      },
    };
    const result = await post("/authentication/verify", json);
    show({ ...result, mediation: mediation ?? "optional", isAssertionResponse: credential.response instanceof AuthenticatorAssertionResponse });
  } catch (error) {
    show({ error: error.name, message: error.message });
  }
}
document.getElementById("register").addEventListener("click", () => register(false));
document.getElementById("register-excluded").addEventListener("click", () => register(true));
document.getElementById("sign-in").addEventListener("click", () => signIn(undefined));
document.getElementById("conditional").addEventListener("click", () => { show("waiting for autofill"); signIn("conditional"); });
</script>
</body></html>`;

export interface PasskeySite {
  origin: string;
  allCredential: (WebAuthnCredential & { aaguid: string })[];
  close: () => Promise<void>;
}

async function readJSON(request: IncomingMessage): Promise<Record<string, unknown>> {
  const allChunk: Buffer[] = [];
  for await (const chunk of request) allChunk.push(chunk as Buffer);
  return JSON.parse(Buffer.concat(allChunk).toString("utf8") || "{}") as Record<string, unknown>;
}

export async function startPasskeySite(): Promise<PasskeySite> {
  const relyingPartyID = "localhost";
  const userID = new TextEncoder().encode("user-alex-0001");
  const allCredential: PasskeySite["allCredential"] = [];
  let expectedChallenge = "";
  let origin = "";
  const server: Server = createServer((request, response) => {
    const answer = (body: unknown) => {
      response.setHeader("Content-Type", "application/json");
      response.end(JSON.stringify(body));
    };
    void (async () => {
      try {
        if (request.method === "GET") {
          response.setHeader("Content-Type", "text/html; charset=utf-8");
          response.end(page);
          return;
        }
        const body = await readJSON(request);
        switch (request.url) {
          case "/registration/options": {
            const options = await generateRegistrationOptions({
              rpName: "Passkey test site",
              rpID: relyingPartyID,
              userName: "alex@example.com",
              userDisplayName: "Alex",
              userID,
              attestationType: "none",
              excludeCredentials: body.isExcluded ? allCredential.map((credential) => ({ id: credential.id })) : [],
              authenticatorSelection: { residentKey: "required", userVerification: "preferred" },
            });
            expectedChallenge = options.challenge;
            return answer(options);
          }
          case "/registration/verify": {
            const verification = await verifyRegistrationResponse({
              response: body as never,
              expectedChallenge,
              expectedOrigin: origin,
              expectedRPID: relyingPartyID,
              requireUserVerification: false,
            });
            const info = verification.registrationInfo;
            if (verification.verified && info) allCredential.push({ ...info.credential, aaguid: info.aaguid });
            return answer({
              verified: verification.verified,
              aaguid: info?.aaguid,
              credentialID: info?.credential.id,
              userVerified: info?.userVerified,
              backedUp: info?.credentialBackedUp,
              fmt: info?.fmt,
            });
          }
          case "/authentication/options": {
            const options = await generateAuthenticationOptions({ rpID: relyingPartyID, userVerification: "preferred" });
            expectedChallenge = options.challenge;
            return answer(options);
          }
          case "/authentication/verify": {
            const credential = allCredential.find((candidate) => candidate.id === body.id);
            if (!credential) return answer({ verified: false, error: "unknown credential" });
            const verification = await verifyAuthenticationResponse({
              response: body as never,
              expectedChallenge,
              expectedOrigin: origin,
              expectedRPID: relyingPartyID,
              credential,
              requireUserVerification: false,
            });
            return answer({ verified: verification.verified, credentialID: credential.id, aaguid: credential.aaguid, userVerified: verification.authenticationInfo.userVerified });
          }
        }
        response.statusCode = 404;
        answer({ error: "not found" });
      } catch (error) {
        response.statusCode = 400;
        answer({ verified: false, error: error instanceof Error ? error.message : String(error) });
      }
    })();
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  origin = `http://localhost:${(server.address() as AddressInfo).port}`;
  return { origin, allCredential, close: () => new Promise<void>((resolve) => {
      server.close(() => resolve());
      server.closeAllConnections();
    }),
  };
}
