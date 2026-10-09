import { passkeyPromptPortName, type PasskeyPromptCall, type PasskeyPromptResponse } from "../shared/passkey";
import { HisarError, isErrorCode } from "../shared/protocol";

export interface PromptChannel {
  call<Result>(call: PasskeyPromptCall): Promise<Result>;
}

export interface PromptPort {
  postMessage(message: unknown): void;
  onMessage: { addListener(listener: (message: unknown) => void): void };
  onDisconnect: { addListener(listener: () => void): void };
}

export function createPromptChannel(port: PromptPort, sessionID: string, createID: () => string): PromptChannel {
  const allPending = new Map<string, { resolve: (result: unknown) => void; reject: (error: HisarError) => void }>();
  let isDisconnected = false;
  port.onMessage.addListener((message) => {
    const response = message as PasskeyPromptResponse;
    const pending = typeof response?.callID === "string" ? allPending.get(response.callID) : undefined;
    if (!pending) return;
    allPending.delete(response.callID);
    if (response.ok) pending.resolve(response.result);
    else pending.reject(new HisarError(isErrorCode(response.error?.code) ? response.error.code : "internal", response.error?.message));
  });
  port.onDisconnect.addListener(() => {
    isDisconnected = true;
    for (const pending of allPending.values()) pending.reject(new HisarError("internal", "This passkey request has ended"));
    allPending.clear();
  });
  return {
    call<Result>(call: PasskeyPromptCall): Promise<Result> {
      if (isDisconnected) return Promise.reject(new HisarError("internal", "This passkey request has ended"));
      const callID = createID();
      return new Promise<Result>((resolve, reject) => {
        allPending.set(callID, { resolve: resolve as (result: unknown) => void, reject });
        port.postMessage({ ...call, callID, sessionID });
      });
    },
  };
}

export function connectPromptChannel(sessionID: string): PromptChannel {
  return createPromptChannel(chrome.runtime.connect({ name: passkeyPromptPortName }), sessionID, () => crypto.randomUUID());
}
