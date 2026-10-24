import {
  allUnauthenticatedType,
  HisarError,
  isErrorCode,
  type HelloResult,
  type RequestMap,
  type RequestType,
  type ResponseFrame,
} from "../shared/protocol";

export interface NativePort {
  postMessage(message: unknown): void;
  disconnect(): void;
  onMessage: { addListener(listener: (message: unknown) => void): void };
  onDisconnect: { addListener(listener: () => void): void };
}

export interface NativeClientOptions {
  connectNative: () => NativePort;
  getLastErrorMessage: () => string | undefined;
  getHelloParameters: () => Promise<RequestMap["hello"]["parameters"]>;
  getToken: () => Promise<string | undefined>;
  createID: () => string;
  requestTimeoutMilliseconds?: number;
  helloTimeoutMilliseconds?: number;
  allReconnectDelayMilliseconds?: number[];
  onHello?: (result: HelloResult) => Promise<void> | void;
}

interface PendingRequest {
  resolve: (result: unknown) => void;
  reject: (error: HisarError) => void;
  timer: ReturnType<typeof setTimeout>;
}

// RequestOptions: pairingToken sends the request only under that pairing, so what it carries from one pairing never goes
// out under the next, as after another account signs in to Hisar; isOverOpenConnection sends it only over the connection
// open now, as connecting starts the host, which relaunches Hisar after the user quit it.
export interface RequestOptions {
  timeoutMilliseconds?: number;
  pairingToken?: string;
  isOverOpenConnection?: boolean;
  isStillValid?: () => boolean;
}

interface Connection {
  port: NativePort;
  ready: Promise<HelloResult>;
  isClosed: boolean;
  isReady: boolean;
}

const defaultReconnectDelayMilliseconds = [1000, 2000, 5000, 10000, 30000];

// The requests that only read, which may be sent again when the connection closes before Hisar answers.
const allRepeatableType: ReadonlySet<RequestType> = new Set<RequestType>([
  "logins.match", "logins.fill", "logins.code", "identities.list", "identities.fill", "cards.list", "cards.fill", "vaults.list",
  "items.list", "items.get", "items.reveal", "items.code", "items.icon", "password.generate", "addresses.list", "addresses.fill",
]);

// DisconnectedError fails a request whose connection closed before Hisar answered.
class DisconnectedError extends HisarError {
  constructor(message: string) {
    super("desktop-unavailable", message);
  }
}
// maximumMessageBytes is browserprotocol.MaxMessageSize: the native host exits on a larger frame.
const maximumMessageBytes = 1 << 20;

export class NativeClient {
  private connection: Connection | undefined;
  private readonly allPending = new Map<string, PendingRequest>();
  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  private lastHello: HelloResult | undefined;

  constructor(private readonly options: NativeClientOptions) {}

  getLastHello(): HelloResult | undefined {
    return this.lastHello;
  }

  isConnected(): boolean {
    return this.connection !== undefined && !this.connection.isClosed;
  }

  async connect(): Promise<HelloResult> {
    if (this.reconnectTimer !== undefined) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    if (this.connection && !this.connection.isClosed) return this.connection.ready;
    let port: NativePort;
    try {
      port = this.options.connectNative();
    } catch (error) {
      throw new HisarError("desktop-unavailable", error instanceof Error ? error.message : String(error));
    }
    const connection: Connection = { port, ready: Promise.resolve() as unknown as Promise<HelloResult>, isClosed: false, isReady: false };
    this.connection = connection;
    port.onMessage.addListener((message) => this.receiveFrame(message));
    port.onDisconnect.addListener(() => this.handleDisconnect(connection));
    connection.ready = (async () => {
      const parameters = await this.options.getHelloParameters();
      const result = await this.send(connection, "hello", parameters, this.options.helloTimeoutMilliseconds ?? 20000);
      this.lastHello = result;
      connection.isReady = true;
      this.reconnectAttempt = 0;
      await this.options.onHello?.(result);
      return result;
    })();
    connection.ready.catch(() => this.closeConnection(connection));
    return connection.ready;
  }

  async reconnect(): Promise<HelloResult> {
    this.disconnect();
    return this.connect();
  }

  // waitForOpenConnection returns the hello of the connection open now, and never opens one.
  async waitForOpenConnection(): Promise<HelloResult> {
    const connection = this.connection;
    if (!connection || connection.isClosed) throw new HisarError("desktop-unavailable", "Hisar is not connected");
    return connection.ready;
  }

  async request<Type extends RequestType>(
    type: Type,
    parameters: RequestMap[Type]["parameters"],
    options: RequestOptions = {},
  ): Promise<RequestMap[Type]["result"]> {
    if (type === "hello") return (await this.reconnect()) as RequestMap[Type]["result"];
    try {
      return await this.sendOnce(type, parameters, options);
    } catch (error) {
      // A request that only reads is sent once more over a new connection when the old one closed under it; one that
      // changes something, or that timed out, is never sent twice.
      if (!allRepeatableType.has(type) || options.isOverOpenConnection || !(error instanceof DisconnectedError)) throw error;
      return this.sendOnce(type, parameters, options);
    }
  }

  private async sendOnce<Type extends RequestType>(
    type: Type,
    parameters: RequestMap[Type]["parameters"],
    options: RequestOptions,
  ): Promise<RequestMap[Type]["result"]> {
    if (options.isOverOpenConnection) await this.waitForOpenConnection();
    else await this.connect();
    const connection = this.connection;
    if (!connection || connection.isClosed) throw new HisarError("desktop-unavailable", "Hisar disconnected");
    return this.send(connection, type, parameters, options.timeoutMilliseconds ?? this.options.requestTimeoutMilliseconds ?? 15000, options.pairingToken, options.isStillValid);
  }

  disconnect(): void {
    if (this.reconnectTimer !== undefined) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    const connection = this.connection;
    if (!connection) return;
    this.closeConnection(connection);
    try {
      connection.port.disconnect();
    } catch {
      // Already disconnected.
    }
  }

  private async send<Type extends RequestType>(
    connection: Connection,
    type: Type,
    parameters: RequestMap[Type]["parameters"],
    timeoutMilliseconds: number,
    pairingToken?: string,
    isStillValid?: () => boolean,
  ): Promise<RequestMap[Type]["result"]> {
    const id = this.options.createID();
    const frame: Record<string, unknown> = { ...parameters, id, type };
    if (!allUnauthenticatedType.includes(type)) {
      const token = await this.options.getToken();
      if (!token) throw new HisarError("not-paired", "This browser is not paired with Hisar");
      // Nothing awaits between this check and postMessage, so the frame goes out under the pairing it was checked against.
      if (pairingToken !== undefined && token !== pairingToken) throw new HisarError("not-paired", "This browser has been paired with Hisar again");
      frame.token = token;
    }
    if (connection.isClosed) throw new HisarError("desktop-unavailable", "Hisar disconnected");
    if (isStillValid && !isStillValid()) throw new HisarError("not-found", "This save offer has expired");
    if (new TextEncoder().encode(JSON.stringify(frame)).length > maximumMessageBytes) throw new HisarError("invalid-request", "This request is too large for Hisar");
    return new Promise<RequestMap[Type]["result"]>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.allPending.delete(id);
        reject(new HisarError("desktop-unavailable", "Hisar did not answer in time"));
      }, timeoutMilliseconds);
      this.allPending.set(id, { resolve: resolve as (result: unknown) => void, reject, timer });
      try {
        connection.port.postMessage(frame);
      } catch (error) {
        clearTimeout(timer);
        this.allPending.delete(id);
        reject(new HisarError("desktop-unavailable", error instanceof Error ? error.message : String(error)));
      }
    });
  }

  private receiveFrame(message: unknown): void {
    if (typeof message !== "object" || message === null) return;
    const frame = message as ResponseFrame;
    if (typeof frame.id !== "string") return;
    const pending = this.allPending.get(frame.id);
    if (!pending) return;
    this.allPending.delete(frame.id);
    clearTimeout(pending.timer);
    if (frame.ok === true) {
      pending.resolve(frame.result ?? {});
      return;
    }
    const code = isErrorCode(frame.error?.code) ? frame.error.code : "internal";
    pending.reject(new HisarError(code, frame.error?.message || code));
  }

  private handleDisconnect(connection: Connection): void {
    const wasReady = connection.isReady && !connection.isClosed;
    const message = this.options.getLastErrorMessage() || "Hisar disconnected";
    this.closeConnection(connection, message);
    if (wasReady) this.scheduleReconnect();
  }

  private closeConnection(connection: Connection, message = "Hisar disconnected"): void {
    if (connection.isClosed) return;
    connection.isClosed = true;
    if (this.connection === connection) this.connection = undefined;
    for (const [id, pending] of this.allPending) {
      clearTimeout(pending.timer);
      pending.reject(new DisconnectedError(message));
      this.allPending.delete(id);
    }
  }

  private scheduleReconnect(): void {
    const allDelay = this.options.allReconnectDelayMilliseconds ?? defaultReconnectDelayMilliseconds;
    if (this.reconnectAttempt >= allDelay.length) return;
    const delay = allDelay[this.reconnectAttempt];
    this.reconnectAttempt += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = undefined;
      this.connect().catch(() => {
        if (!this.isConnected()) this.scheduleReconnect();
      });
    }, delay);
  }
}
