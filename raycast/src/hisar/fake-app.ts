import { mkdtempSync, rmSync } from "node:fs";
import { createServer, type Socket } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { encodeFrame, FrameReader } from "./framing";
import { allLauncherRequestType } from "./protocol";

export type ReceivedRequest = { id: string; type: string } & Record<string, unknown>;

/** Answer returns the whole response frame for a request, now or later, or undefined to never answer it. */
export type Answer = (request: ReceivedRequest) => unknown;

/** FakeApp is a Unix-socket server that speaks cli.sock framing, records every request and answers as told. */
export interface FakeApp {
  allRequest: ReceivedRequest[];
  getConnectionCount(): number;
  getRequestTypes(): string[];
  close(): Promise<void>;
}

/** answerHandshake answers hello with allFeature and launcher.hello as approved; other requests go to answer. */
export function answerHandshake(answer: Answer, allFeature: readonly string[] = allLauncherRequestType): Answer {
  return (request) => {
    if (request.type === "hello") {
      return { id: request.id, ok: true, result: { protocolVersion: 1, state: "unlocked", features: allFeature } };
    }
    if (request.type === "launcher.hello") return { id: request.id, ok: true, result: { isApproved: true } };
    return answer(request);
  };
}

export interface SocketDirectory {
  socketPath: string;
  remove(): void;
}

/** createSocketDirectory makes a private temporary folder for a socket and removes it after the test. */
export function createSocketDirectory(): SocketDirectory {
  const directory = mkdtempSync(join(tmpdir(), "hisar-raycast-"));
  return { socketPath: join(directory, "cli.sock"), remove: () => rmSync(directory, { recursive: true, force: true }) };
}

export async function startFakeApp(socketPath: string, answer: Answer): Promise<FakeApp> {
  const allRequest: ReceivedRequest[] = [];
  const allSocket = new Set<Socket>();
  let connectionCount = 0;
  const server = createServer((socket) => {
    connectionCount += 1;
    allSocket.add(socket);
    socket.on("close", () => allSocket.delete(socket));
    const reader = new FrameReader();
    socket.on("data", (chunk: Buffer) => {
      for (const message of reader.push(chunk)) {
        const request = message as ReceivedRequest;
        allRequest.push(request);
        void Promise.resolve(answer(request)).then((response) => {
          if (response !== undefined && !socket.destroyed) socket.write(encodeFrame(response));
        });
      }
    });
  });
  const { promise: listening, resolve, reject } = Promise.withResolvers<void>();
  server.once("error", reject);
  server.listen(socketPath, resolve);
  await listening;
  return {
    allRequest,
    getConnectionCount: () => connectionCount,
    getRequestTypes: () => allRequest.map((request) => request.type),
    close: async () => {
      for (const socket of allSocket) socket.destroy();
      const { promise: closed, resolve: resolveClosed } = Promise.withResolvers<void>();
      server.close(() => resolveClosed());
      await closed;
    },
  };
}
