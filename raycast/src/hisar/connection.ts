import type { Socket } from "node:net";
import { HisarError } from "./errors";
import { encodeFrame, FrameReader } from "./framing";
import {
  clientPlatform,
  parseResponse,
  parseResultByType,
  protocolVersion,
  type LauncherRequestType,
  type RequestMap,
  type RequestType,
} from "./protocol";

// A request that may open Hisar's approval window waits for it: 60 s, or 2 minutes when the iPhone can answer.
const approvalTimeoutMilliseconds = 150_000;

const timeoutMillisecondsByType: Record<RequestType, number> = {
  hello: 5_000,
  "launcher.hello": 10_000,
  "launcher.items.list": approvalTimeoutMilliseconds,
  "launcher.items.code": approvalTimeoutMilliseconds,
  "launcher.items.password": approvalTimeoutMilliseconds,
  "launcher.items.copy": approvalTimeoutMilliseconds,
  "launcher.passwords.generate": 10_000,
  "launcher.lock": 10_000,
};

/** Connection is one cli.sock connection that said hello and launcher.hello; requests go one at a time. */
export class Connection {
  readonly isApproved: boolean;
  private readonly channel: RequestChannel;
  private readonly allFeature: string[];

  private constructor(channel: RequestChannel, allFeature: string[], isApproved: boolean) {
    this.channel = channel;
    this.allFeature = allFeature;
    this.isApproved = isApproved;
  }

  /** open says hello as darwin/raycast, then launcher.hello; a Hisar that does not announce launcher.hello needs an update. */
  static async open(socket: Socket, clientVersion: string): Promise<Connection> {
    const channel = new RequestChannel(socket);
    try {
      const hello = await channel.send("hello", { clientPlatform, clientVersion });
      if (hello.protocolVersion !== protocolVersion) {
        throw new HisarError(
          "update-required",
          `Hisar speaks protocol version ${hello.protocolVersion} and this extension ${protocolVersion}. Update Hisar and the extension.`,
        );
      }
      requireFeature(hello.allFeature, "launcher.hello");
      const { isApproved } = await channel.send("launcher.hello", {});
      return new Connection(channel, hello.allFeature, isApproved);
    } catch (error) {
      channel.close();
      throw error;
    }
  }

  get isClosed(): boolean {
    return this.channel.isClosed;
  }

  /** call sends type only when Hisar announced it; an older Hisar gets update-required and no request. */
  call<Type extends LauncherRequestType>(
    type: Type,
    parameters: RequestMap[Type]["parameters"],
  ): Promise<RequestMap[Type]["result"]> {
    requireFeature(this.allFeature, type);
    return this.channel.send(type, parameters);
  }

  close(): void {
    this.channel.close();
  }
}

function requireFeature(allFeature: string[], type: LauncherRequestType): void {
  if (!allFeature.includes(type)) {
    throw new HisarError("update-required", "This version of Hisar cannot answer this command. Update Hisar and try again.");
  }
}

interface Exchange {
  id: string;
  resolve: (result: unknown) => void;
  reject: (error: HisarError) => void;
  timer: NodeJS.Timeout;
}

/** RequestChannel owns the socket: request IDs, the one exchange in flight, timeouts and the end of the connection. */
class RequestChannel {
  private readonly socket: Socket;
  private readonly reader = new FrameReader();
  private lastID = 0;
  private queue: Promise<unknown> = Promise.resolve();
  private exchange: Exchange | undefined;
  private endError: HisarError | undefined;

  constructor(socket: Socket) {
    this.socket = socket;
    socket.on("data", (chunk: Buffer) => this.receive(chunk));
    socket.on("error", (error) => this.end(new HisarError("connection-lost", `Lost the connection to Hisar: ${error.message}`)));
    socket.on("close", () => this.end(new HisarError("connection-lost", "Hisar closed the connection.")));
  }

  get isClosed(): boolean {
    return this.endError !== undefined;
  }

  send<Type extends RequestType>(type: Type, parameters: RequestMap[Type]["parameters"]): Promise<RequestMap[Type]["result"]> {
    const sent = this.queue.then(async () => parseResultByType[type](await this.exchangeOne(type, parameters)));
    this.queue = sent.catch(() => undefined);
    return sent;
  }

  close(): void {
    this.end(new HisarError("connection-lost", "The connection to Hisar is closed."));
  }

  private exchangeOne(type: RequestType, parameters: object): Promise<unknown> {
    if (this.endError) return Promise.reject(this.endError);
    this.lastID += 1;
    const id = String(this.lastID);
    const frame = encodeFrame({ id, type, ...parameters });
    const { promise, resolve, reject } = Promise.withResolvers<unknown>();
    const timer = setTimeout(
      () => this.end(new HisarError("timeout", "Hisar did not answer in time.")),
      timeoutMillisecondsByType[type],
    );
    this.exchange = { id, resolve, reject, timer };
    this.socket.write(frame);
    return promise;
  }

  private receive(chunk: Buffer): void {
    try {
      for (const message of this.reader.push(chunk)) {
        const response = parseResponse(message);
        const exchange = this.exchange;
        if (exchange === undefined || response.id !== exchange.id) {
          throw new HisarError("connection-lost", "Hisar answered a request this extension did not send.");
        }
        clearTimeout(exchange.timer);
        this.exchange = undefined;
        if ("error" in response) exchange.reject(response.error);
        else exchange.resolve(response.result);
      }
    } catch (error) {
      this.end(error instanceof HisarError ? error : new HisarError("connection-lost", String(error)));
    }
  }

  // After a timeout or a broken stream no later answer can be paired with its request, so the connection ends.
  private end(error: HisarError): void {
    if (this.endError) return;
    this.endError = error;
    const exchange = this.exchange;
    this.exchange = undefined;
    if (exchange) {
      clearTimeout(exchange.timer);
      exchange.reject(error);
    }
    this.socket.destroy();
  }
}
