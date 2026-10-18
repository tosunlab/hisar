export interface SignInResult {
  token: string;
  accountId: string;
  deviceId: string;
  role: string;
  name: string;
}

// ke3 is base64: Go's encoding/json encodes apitypes.ReauthenticationProof.KE3, a []byte, that way.
export interface ReauthenticationProof {
  reauthenticationId: string;
  ke3: string;
}

export type BridgeRequest =
  | { type: "signIn"; email: string; secretKey: string; password: string }
  | { type: "reauthenticate"; token: string; email: string; secretKey: string; password: string };

export interface BridgeResult {
  signIn: SignInResult;
  reauthenticate: ReauthenticationProof;
}

export type WorkerRequest = BridgeRequest & { id: number };

export type WorkerResponse = { id: number; value: BridgeResult[keyof BridgeResult] } | { id: number; error: string };

export interface BridgeWorker {
  onmessage: ((event: MessageEvent<WorkerResponse>) => void) | null;
  onerror: ((event: ErrorEvent) => void) | null;
  postMessage(request: WorkerRequest): void;
  terminate(): void;
}

interface PendingCall {
  resolve(value: BridgeResult[keyof BridgeResult]): void;
  reject(error: Error): void;
}

export class Bridge {
  private worker: BridgeWorker | null = null;
  private nextID = 1;
  private readonly allPendingCall = new Map<number, PendingCall>();

  constructor(private readonly createWorker: () => BridgeWorker) {}

  start(): BridgeWorker {
    if (this.worker) {
      return this.worker;
    }
    const worker = this.createWorker();
    worker.onmessage = (event) => this.settle(event.data);
    worker.onerror = (event) => {
      event.preventDefault();
      this.stop(new Error(event.message || "The admin dashboard could not load Hisar's encryption code. Reload the page."));
    };
    this.worker = worker;
    return worker;
  }

  signIn(email: string, secretKey: string, password: string): Promise<SignInResult> {
    return this.call({ type: "signIn", email, secretKey, password });
  }

  reauthenticate(token: string, email: string, secretKey: string, password: string): Promise<ReauthenticationProof> {
    return this.call({ type: "reauthenticate", token, email, secretKey, password });
  }

  private call<Type extends BridgeRequest["type"]>(request: Extract<BridgeRequest, { type: Type }>): Promise<BridgeResult[Type]> {
    const worker = this.start();
    const id = this.nextID++;
    const { promise, resolve, reject } = Promise.withResolvers<BridgeResult[Type]>();
    this.allPendingCall.set(id, { resolve: resolve as PendingCall["resolve"], reject });
    worker.postMessage({ ...request, id });
    return promise;
  }

  private settle(response: WorkerResponse): void {
    const pendingCall = this.allPendingCall.get(response.id);
    this.allPendingCall.delete(response.id);
    if ("error" in response) {
      pendingCall?.reject(new Error(response.error));
    } else {
      pendingCall?.resolve(response.value);
    }
  }

  private stop(error: Error): void {
    this.worker?.terminate();
    this.worker = null;
    for (const pendingCall of this.allPendingCall.values()) {
      pendingCall.reject(error);
    }
    this.allPendingCall.clear();
  }
}

export const bridge = new Bridge(() => new Worker(new URL("./worker.ts", import.meta.url), { type: "module", name: "hisar-crypto" }));
