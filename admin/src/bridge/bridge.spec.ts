import { describe, expect, it } from "vitest";
import { Bridge, type BridgeWorker, type SignInResult, type WorkerRequest, type WorkerResponse } from "./bridge";

class FakeWorker implements BridgeWorker {
  onmessage: ((event: MessageEvent<WorkerResponse>) => void) | null = null;
  onerror: ((event: ErrorEvent) => void) | null = null;
  allRequest: WorkerRequest[] = [];
  isTerminated = false;

  postMessage(request: WorkerRequest): void {
    this.allRequest.push(request);
  }

  terminate(): void {
    this.isTerminated = true;
  }

  answer(response: WorkerResponse): void {
    this.onmessage?.({ data: response } as MessageEvent<WorkerResponse>);
  }

  fail(message: string): void {
    this.onerror?.({ message, preventDefault() {} } as ErrorEvent);
  }
}

function createSignIn(name: string): SignInResult {
  return { token: `token-${name}`, accountId: `account-${name}`, deviceId: `device-${name}`, role: "admin", name };
}

describe("Bridge", () => {
  it("sends typed requests and resolves two calls in flight, each with its own answer", async () => {
    const allWorker: FakeWorker[] = [];
    const bridge = new Bridge(() => allWorker[allWorker.push(new FakeWorker()) - 1]);
    const first = bridge.signIn("ada@example.com", "H1-A", "first password");
    const second = bridge.reauthenticate("token", "ada@example.com", "H1-A", "second password");
    expect(allWorker).toHaveLength(1);
    const [worker] = allWorker;
    expect(worker.allRequest).toEqual([
      { type: "signIn", email: "ada@example.com", secretKey: "H1-A", password: "first password", id: 1 },
      { type: "reauthenticate", token: "token", email: "ada@example.com", secretKey: "H1-A", password: "second password", id: 2 },
    ]);
    const settledOrder: string[] = [];
    void first.then(() => settledOrder.push("first"));
    void second.then(() => settledOrder.push("second"));

    worker.answer({ id: 1, value: createSignIn("Ada") });
    await expect(first).resolves.toEqual(createSignIn("Ada"));
    worker.answer({ id: 2, value: { reauthenticationId: "r1", ke3: "a2Uz" } });
    await expect(second).resolves.toEqual({ reauthenticationId: "r1", ke3: "a2Uz" });
    expect(settledOrder).toEqual(["first", "second"]);
  });

  it("matches answers to calls by id when they arrive out of order", async () => {
    const worker = new FakeWorker();
    const bridge = new Bridge(() => worker);
    const first = bridge.signIn("ada@example.com", "H1-A", "password");
    const second = bridge.signIn("bob@example.com", "H1-B", "password");
    worker.answer({ id: 2, value: createSignIn("Bob") });
    worker.answer({ id: 1, value: createSignIn("Ada") });
    await expect(first).resolves.toMatchObject({ name: "Ada" });
    await expect(second).resolves.toMatchObject({ name: "Bob" });
  });

  it("rejects a call with the worker's error message", async () => {
    const worker = new FakeWorker();
    const bridge = new Bridge(() => worker);
    const call = bridge.signIn("ada@example.com", "H1-A", "wrong password");
    worker.answer({ id: 1, error: "incorrect email, master password or Secret Key" });
    await expect(call).rejects.toThrow("incorrect email, master password or Secret Key");
  });

  it("rejects every pending call when the worker fails, and starts a new worker for the next call", async () => {
    const allWorker: FakeWorker[] = [];
    const bridge = new Bridge(() => allWorker[allWorker.push(new FakeWorker()) - 1]);
    const first = bridge.signIn("ada@example.com", "H1-A", "password");
    const second = bridge.reauthenticate("token", "ada@example.com", "H1-A", "password");
    allWorker[0].fail("Uncaught CompileError: WebAssembly.instantiate(): expected magic word");
    await expect(first).rejects.toThrow("expected magic word");
    await expect(second).rejects.toThrow("expected magic word");
    expect(allWorker[0].isTerminated).toBe(true);

    const third = bridge.signIn("ada@example.com", "H1-A", "password");
    expect(allWorker).toHaveLength(2);
    allWorker[1].answer({ id: 3, value: createSignIn("Ada") });
    await expect(third).resolves.toMatchObject({ name: "Ada" });
  });

  it("says what failed when the worker's error has no message", async () => {
    const worker = new FakeWorker();
    const bridge = new Bridge(() => worker);
    const call = bridge.signIn("ada@example.com", "H1-A", "password");
    worker.fail("");
    await expect(call).rejects.toThrow("The admin dashboard could not load Hisar's encryption code. Reload the page.");
  });
});
