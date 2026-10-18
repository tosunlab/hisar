import "../../wasm/wasm_exec.js";
import wasmURL from "../../wasm/hisar.wasm?url";
import type { BridgeRequest, BridgeResult, WorkerRequest, WorkerResponse } from "./bridge";

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

declare const hisar: {
  signIn(email: string, secretKey: string, password: string): Promise<BridgeResult["signIn"]>;
  reauthenticate(token: string, email: string, secretKey: string, password: string): Promise<BridgeResult["reauthenticate"]>;
};

// The Go program sets globalThis.hisar before it first blocks, so it is ready once run has started.
const ready = (async () => {
  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch(wasmURL), go.importObject);
  void go.run(instance);
})();

function callHisar(request: BridgeRequest): Promise<BridgeResult[keyof BridgeResult]> {
  switch (request.type) {
    case "signIn":
      return hisar.signIn(request.email, request.secretKey, request.password);
    case "reauthenticate":
      return hisar.reauthenticate(request.token, request.email, request.secretKey, request.password);
  }
}

self.onmessage = async (event: MessageEvent<WorkerRequest>) => {
  const { id } = event.data;
  let response: WorkerResponse;
  try {
    await ready;
    response = { id, value: await callHisar(event.data) };
  } catch (error) {
    response = { id, error: error instanceof Error ? error.message : String(error) };
  }
  self.postMessage(response);
};
