import "../../wasm/wasm_exec.js";
import wasmURL from "../../wasm/hisar.wasm?url";
import type { BridgeRequest, BridgeResult, WorkerRequest, WorkerResponse } from "./bridge";
import { stageRecovery, loadRecovery, listRecovery, removeRecovery } from "./recoveryStorage";

declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

declare const hisar: {
  signIn(email: string, secretKey: string, password: string): Promise<BridgeResult["signIn"]>;
  reauthenticate(token: string, email: string, secretKey: string, password: string, action?: string, targetAccountID?: string, planVersion?: string): Promise<BridgeResult["reauthenticate"]>;
  signUp(inviteToken: string, email: string, name: string, password: string): Promise<BridgeResult["signUp"]>;
  listPendingRecovery(): Promise<BridgeResult["listPendingRecovery"]>;
  resumePendingRecoveryWithSecretKey(operationId: string, password: string, secretKey: string, priorSecretKey: string): Promise<BridgeResult["resumePendingRecoveryWithSecretKey"]>;
  exportPendingRecoveryEmergencyKit(operationId: string, password: string): Promise<BridgeResult["exportPendingRecoveryEmergencyKit"]>;
  acknowledgePendingRecovery(operationId: string, password: string): Promise<void>;
  discardPreparedRecovery(operationId: string, password: string): Promise<void>;
};

Object.assign(globalThis, {
  stageHisarRecovery: stageRecovery,
  loadHisarRecovery: loadRecovery,
  listHisarRecovery: listRecovery,
  removeHisarRecovery: removeRecovery,
});
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
      return hisar.reauthenticate(request.token, request.email, request.secretKey, request.password, request.action, request.targetAccountID, request.planVersion);
    case "signUp":
      return hisar.signUp(request.inviteToken, request.email, request.name, request.password);
    case "listPendingRecovery":
      return hisar.listPendingRecovery();
    case "resumePendingRecoveryWithSecretKey":
      return hisar.resumePendingRecoveryWithSecretKey(request.operationId, request.password, request.secretKey, request.priorSecretKey);
    case "exportPendingRecoveryEmergencyKit":
      return hisar.exportPendingRecoveryEmergencyKit(request.operationId, request.password);
    case "acknowledgePendingRecovery":
      return hisar.acknowledgePendingRecovery(request.operationId, request.password);
    case "discardPreparedRecovery":
      return hisar.discardPreparedRecovery(request.operationId, request.password);
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
