import { Connection } from "./connection";
import { HisarError } from "./errors";
import { dialHisar, resolveSocketPath, systemLaunch, type LaunchSystem } from "./launch";
import type {
  CopyReceipt,
  CopyableField,
  ItemList,
  ItemReference,
  OneTimeCode,
  OpenOptions,
  PasswordOptions,
  ShareLinkOptions,
} from "./model";

export { HisarError, toHisarError, type ErrorCode } from "./errors";
export { openHisar, type LaunchSystem } from "./launch";
export type * from "./model";

export interface ConnectOptions {
  /** socketPath is Hisar's command-line socket; empty means ~/.hisar/cli.sock, and ~/ starts in the home folder. */
  socketPath?: string;
  /** clientVersion is the extension's version, which hello tells Hisar. */
  clientVersion: string;
  launchSystem?: LaunchSystem;
}

/**
 * Hisar is the app as a launcher sees it: item metadata, one secret at a time, copies made by the app. It connects on
 * the first request, starting Hisar hidden when it is not running (except to lock it), and again after the connection
 * ended. Requests go one at a time, in the order they were made.
 */
export class Hisar {
  private readonly options: ConnectOptions;
  private connection: Connection | undefined;
  private queue: Promise<unknown> = Promise.resolve();
  private closeCount = 0;

  constructor(options: ConnectOptions) {
    this.options = options;
  }

  /** isApproved tells whether Hisar lets this launcher in already, or its next request opens the approval window. */
  isApproved(): Promise<boolean> {
    return this.run(true, async (connection) => connection.isApproved);
  }

  listItems(): Promise<ItemList> {
    return this.run(true, (connection) => connection.call("launcher.items.list", {}));
  }

  getCode(item: ItemReference): Promise<OneTimeCode> {
    return this.run(true, (connection) =>
      connection.call("launcher.items.code", { vaultID: item.vaultID, itemID: item.itemID }),
    );
  }

  /** readPassword returns one item's password, for one paste. */
  readPassword(item: ItemReference): Promise<string> {
    return this.run(true, (connection) =>
      connection.call("launcher.items.password", { vaultID: item.vaultID, itemID: item.itemID }),
    );
  }

  /** copy has Hisar copy the field; the value never reaches the extension. */
  copy(item: ItemReference, field: CopyableField): Promise<CopyReceipt> {
    return this.run(true, (connection) =>
      connection.call("launcher.items.copy", { vaultID: item.vaultID, itemID: item.itemID, field }),
    );
  }

  /** open shows the item in Hisar's window, or its editor; it starts Hisar when it is not running. */
  open(item: ItemReference, options: OpenOptions = {}): Promise<void> {
    const parameters = {
      vaultID: item.vaultID,
      itemID: item.itemID,
      ...(options.isEditing && { isEditing: true as const }),
    };
    return this.run(true, (connection) => connection.call("launcher.items.open", parameters));
  }

  /** share has Hisar create a share link of the item's fields, which it asks the user every time, and returns its URL. */
  share(item: ItemReference, options: ShareLinkOptions): Promise<string> {
    const shareLink = {
      fields: options.allField,
      expiresInSeconds: options.expiresInSeconds,
      maxViews: options.maxViews,
    };
    return this.run(true, (connection) =>
      connection.call("launcher.items.share", { vaultID: item.vaultID, itemID: item.itemID, shareLink }),
    );
  }

  /** generatePassword uses Hisar's generator; without options, its defaults. */
  generatePassword(options?: PasswordOptions): Promise<string> {
    const parameters = options
      ? { generator: { length: options.length, digits: options.digits, symbols: options.symbols } }
      : {};
    return this.run(true, (connection) => connection.call("launcher.passwords.generate", parameters));
  }

  /** lock locks Hisar; it never starts Hisar just to lock it. */
  lock(): Promise<void> {
    return this.run(false, (connection) => connection.call("launcher.lock", {}));
  }

  /** close ends the connection and fails in-flight and queued requests; a later request connects again. */
  close(): void {
    this.closeCount += 1;
    this.connection?.close();
    this.connection = undefined;
  }

  // One request at a time: a connection pairs an answer with the only request in flight.
  private run<Result>(isLaunchAllowed: boolean, send: (connection: Connection) => Promise<Result>): Promise<Result> {
    const closeCount = this.closeCount;
    const result = this.queue.then(async () => {
      if (closeCount !== this.closeCount) {
        throw new HisarError("connection-lost", "The connection to Hisar is closed.");
      }
      return send(await this.connect(isLaunchAllowed));
    });
    this.queue = result.catch(() => undefined);
    return result;
  }

  private async connect(isLaunchAllowed: boolean): Promise<Connection> {
    if (this.connection && !this.connection.isClosed) return this.connection;
    const closeCount = this.closeCount;
    const socketPath = resolveSocketPath(this.options.socketPath);
    const socket = await dialHisar(socketPath, isLaunchAllowed, this.options.launchSystem ?? systemLaunch);
    const connection = await Connection.open(socket, this.options.clientVersion);
    // close() ran while this connection was opening; keeping it would leave a socket nobody closes.
    if (closeCount !== this.closeCount) {
      connection.close();
      throw new HisarError("connection-lost", "The connection to Hisar is closed.");
    }
    this.connection = connection;
    return connection;
  }
}

/** withHisar runs action with a Hisar and always closes its connection: for no-view commands. */
export async function withHisar<Result>(
  options: ConnectOptions,
  action: (hisar: Hisar) => Promise<Result>,
): Promise<Result> {
  const hisar = new Hisar(options);
  try {
    return await action(hisar);
  } finally {
    hisar.close();
  }
}
