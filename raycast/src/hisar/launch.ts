import { execFile } from "node:child_process";
import { createConnection, type Socket } from "node:net";
import { homedir } from "node:os";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { promisify } from "node:util";
import { HisarError } from "./errors";

const appBundleID = "com.tryhisar.hisar";
const launchTimeoutMilliseconds = 10_000;
const launchPollIntervalMilliseconds = 200;

/** LaunchSystem starts Hisar when nothing answers on its socket; tests replace it. */
export interface LaunchSystem {
  /** openApp starts Hisar hidden; it rejects when Hisar is not installed. */
  openApp(): Promise<void>;
  /** isAppRunning finds a Hisar whose command-line socket is off: its SSH agent still answers. */
  isAppRunning(): Promise<boolean>;
}

const runFile = promisify(execFile);

/** systemLaunch starts Hisar the way hisar does (cli/cmd/hisar/launch_darwin.go); Raycast has no PATH, hence /usr/bin. */
export const systemLaunch: LaunchSystem = {
  openApp: async () => {
    await runFile("/usr/bin/open", ["-g", "-b", appBundleID, "--args", "--hidden"]);
  },
  isAppRunning: async () => {
    try {
      (await dial(join(homedir(), ".hisar", "agent.sock"))).destroy();
      return true;
    } catch {
      return false;
    }
  },
};

/** openHisar brings Hisar's window forward. */
export async function openHisar(): Promise<void> {
  await runFile("/usr/bin/open", ["-b", appBundleID]);
}

/** resolveSocketPath reads the socket preference: empty means ~/.hisar/cli.sock, and a leading ~ the home folder. */
export function resolveSocketPath(path: string | undefined, homeDirectory: string = homedir()): string {
  const trimmed = path?.trim() ?? "";
  if (trimmed === "") return join(homeDirectory, ".hisar", "cli.sock");
  if (trimmed.startsWith("~/")) return join(homeDirectory, trimmed.slice(2));
  return trimmed;
}

/**
 * dialHisar connects to socketPath. When nothing listens there it starts Hisar hidden, if isLaunchAllowed, and tries
 * again every 200 ms for 10 s, as hisar does (cli/cmd/hisar/launch.go). A running Hisar without a socket has the
 * integration switch off, and starting it again would only show its window.
 */
export async function dialHisar(socketPath: string, isLaunchAllowed: boolean, system: LaunchSystem): Promise<Socket> {
  const socket = await dialUnlessUnreachable(socketPath);
  if (socket) return socket;
  if (await system.isAppRunning()) {
    throw new HisarError(
      "integration-disabled",
      "Turn on Settings → Developer tools → Let the command-line tool use Hisar.",
    );
  }
  if (!isLaunchAllowed) throw new HisarError("app-not-running", "Hisar is not running.");
  try {
    await system.openApp();
  } catch {
    throw new HisarError("app-not-installed", "Install the Hisar app, then try again.");
  }
  const deadline = Date.now() + launchTimeoutMilliseconds;
  while (Date.now() < deadline) {
    await sleep(launchPollIntervalMilliseconds);
    const launched = await dialUnlessUnreachable(socketPath);
    if (launched) return launched;
  }
  throw new HisarError("app-not-running", "Hisar did not start. Open Hisar and try again.");
}

// A missing socket file or a refused connection is the case where starting Hisar helps.
async function dialUnlessUnreachable(socketPath: string): Promise<Socket | undefined> {
  try {
    return await dial(socketPath);
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ENOENT" || code === "ECONNREFUSED") return undefined;
    throw new HisarError("connection-lost", `Could not reach Hisar at ${socketPath}: ${(error as Error).message}`);
  }
}

function dial(socketPath: string): Promise<Socket> {
  const { promise, resolve, reject } = Promise.withResolvers<Socket>();
  const socket = createConnection(socketPath);
  socket.once("connect", () => {
    socket.off("error", reject);
    resolve(socket);
  });
  socket.once("error", reject);
  return promise;
}
