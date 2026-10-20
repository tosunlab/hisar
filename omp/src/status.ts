export type HisarState = "unlocked" | "locked" | "not-running" | "signed-out";

// not-installed: no hisar was found. unavailable: hisar answered without a state this plugin knows, as an older
// hisar without `status --json` does.
export type HisarStatus = HisarState | "not-installed" | "unavailable";

export const statusTexts: Record<HisarStatus, string> = {
  unlocked: "Hisar unlocked",
  locked: "Hisar locked",
  "not-running": "Hisar not running",
  "signed-out": "Hisar signed out",
  "not-installed": "Hisar not installed",
  unavailable: "Hisar status unavailable",
};

export interface CommandResult {
  code: number;
  stdout: string;
}

export type RunCommand = (argv: string[], environment: Record<string, string | undefined>) => Promise<CommandResult>;

const probeTimeoutMilliseconds = 10_000;

export const runCommand: RunCommand = async (argv, environment) => {
  const child = Bun.spawn(argv, {
    env: environment,
    stdin: "ignore",
    stdout: "pipe",
    stderr: "ignore",
    timeout: probeTimeoutMilliseconds,
  });
  const [stdout, code] = await Promise.all([new Response(child.stdout).text(), child.exited]);
  return { code, stdout };
};

const hisarStates: Record<HisarState, true> = { unlocked: true, locked: true, "not-running": true, "signed-out": true };

function parseState(stdout: string): HisarStatus {
  try {
    const state: unknown = JSON.parse(stdout).state;
    return typeof state === "string" && Object.hasOwn(hisarStates, state) ? (state as HisarState) : "unavailable";
  } catch {
    return "unavailable";
  }
}

// probeStatus asks hisar for its state with HISAR_NO_LAUNCH=1, so a probe never starts the Hisar app.
export async function probeStatus(
  hisarPath: string | undefined,
  environment: Record<string, string | undefined>,
  run: RunCommand = runCommand,
): Promise<HisarStatus> {
  if (hisarPath === undefined) return "not-installed";
  try {
    const result = await run([hisarPath, "status", "--json"], { ...environment, HISAR_NO_LAUNCH: "1" });
    return result.code === 0 ? parseState(result.stdout) : "unavailable";
  } catch {
    return "unavailable";
  }
}
