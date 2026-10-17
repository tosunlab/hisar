import { Connection } from "./connection";
import { dialHisar, resolveSocketPath, systemLaunch, type LaunchSystem } from "./launch";
import type { CopyReceipt, CopyableField, ItemList, ItemReference, OneTimeCode, PasswordOptions } from "./model";

export { allErrorCode, HisarError, toHisarError, type ErrorCode } from "./errors";
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
 * the first request, starting Hisar hidden when it is not running (except to lock it), and again after the app closed
 * the connection.
 */
export class Hisar {
  private readonly options: ConnectOptions;
  private connection: Promise<Connection> | undefined;

  constructor(options: ConnectOptions) {
    this.options = options;
  }

  /** isApproved tells whether Hisar lets this launcher in already, or its next request opens the approval window. */
  async isApproved(): Promise<boolean> {
    return (await this.connect(true)).isApproved;
  }

  async listItems(): Promise<ItemList> {
    return (await this.connect(true)).call("launcher.items.list", {});
  }

  async getCode(item: ItemReference): Promise<OneTimeCode> {
    return (await this.connect(true)).call("launcher.items.code", { vaultID: item.vaultID, itemID: item.itemID });
  }

  /** readPassword returns one item's password, for one paste. */
  async readPassword(item: ItemReference): Promise<string> {
    return (await this.connect(true)).call("launcher.items.password", { vaultID: item.vaultID, itemID: item.itemID });
  }

  /** copy has Hisar copy the field; the value never reaches the extension. */
  async copy(item: ItemReference, field: CopyableField): Promise<CopyReceipt> {
    return (await this.connect(true)).call("launcher.items.copy", { vaultID: item.vaultID, itemID: item.itemID, field });
  }

  /** generatePassword uses Hisar's generator; without options, its defaults. */
  async generatePassword(options?: PasswordOptions): Promise<string> {
    const generator = options && { length: options.length, digits: options.digits, symbols: options.symbols };
    return (await this.connect(true)).call("launcher.passwords.generate", generator ? { generator } : {});
  }

  /** lock locks Hisar; it never starts Hisar just to lock it. */
  async lock(): Promise<void> {
    await (await this.connect(false)).call("launcher.lock", {});
  }

  close(): void {
    const connection = this.connection;
    this.connection = undefined;
    connection?.then(
      (opened) => opened.close(),
      () => undefined,
    );
  }

  private async connect(isLaunchAllowed: boolean): Promise<Connection> {
    const current = this.connection;
    if (current) {
      const connection = await current;
      if (!connection.isClosed) return connection;
      if (this.connection === current) this.connection = undefined;
      return this.connect(isLaunchAllowed);
    }
    const opening = this.open(isLaunchAllowed);
    this.connection = opening;
    opening.catch(() => {
      if (this.connection === opening) this.connection = undefined;
    });
    return opening;
  }

  private async open(isLaunchAllowed: boolean): Promise<Connection> {
    const socketPath = resolveSocketPath(this.options.socketPath);
    const socket = await dialHisar(socketPath, isLaunchAllowed, this.options.launchSystem ?? systemLaunch);
    return Connection.open(socket, this.options.clientVersion);
  }
}

/** withHisar runs action with a Hisar and always closes its connection: for no-view commands. */
export async function withHisar<Result>(options: ConnectOptions, action: (hisar: Hisar) => Promise<Result>): Promise<Result> {
  const hisar = new Hisar(options);
  try {
    return await action(hisar);
  } finally {
    hisar.close();
  }
}
