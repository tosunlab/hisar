import { createEventTransport, installPasskeyWrapper, type PasskeyGlobalScope } from "./main-world";

function createRequestID(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

const scope = window as unknown as PasskeyGlobalScope;
installPasskeyWrapper(scope, createEventTransport(document, scope), createRequestID);
