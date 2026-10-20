import { describe, expect, test } from "bun:test";
import { chmodSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type CommandResult, probeStatus, type RunCommand, statusTexts } from "../src/status";

function answering(result: CommandResult): {
  run: RunCommand;
  calls: { argv: string[]; environment: Record<string, string | undefined> }[];
} {
  const calls: { argv: string[]; environment: Record<string, string | undefined> }[] = [];
  return {
    calls,
    run: async (argv, environment) => {
      calls.push({ argv, environment });
      return result;
    },
  };
}

async function probeText(result: CommandResult): Promise<string> {
  return statusTexts[await probeStatus("/bin/hisar", {}, answering(result).run)];
}

describe("probeStatus", () => {
  test("maps each state to its status text", async () => {
    const texts: Record<string, string> = {
      unlocked: "Hisar unlocked",
      locked: "Hisar locked",
      "not-running": "Hisar not running",
      "signed-out": "Hisar signed out",
    };
    for (const [state, text] of Object.entries(texts)) {
      const stdout = JSON.stringify({ mode: "app", state, account: "alice@example.com" });
      expect(await probeText({ code: 0, stdout })).toBe(text);
    }
  });

  test("says Hisar is not installed when no hisar was found", async () => {
    const { run, calls } = answering({ code: 0, stdout: "" });
    expect(statusTexts[await probeStatus(undefined, {}, run)]).toBe("Hisar not installed");
    expect(calls).toEqual([]);
  });

  test("says the status is unavailable when hisar fails or answers something else", async () => {
    expect(await probeText({ code: 2, stdout: "" })).toBe("Hisar status unavailable");
    expect(await probeText({ code: 0, stdout: "mode: Hisar app" })).toBe("Hisar status unavailable");
    expect(await probeText({ code: 0, stdout: '{"state":"constructor"}' })).toBe("Hisar status unavailable");
    const failing: RunCommand = async () => {
      throw new Error("spawn failed");
    };
    expect(statusTexts[await probeStatus("/bin/hisar", {}, failing)]).toBe("Hisar status unavailable");
  });

  test("runs hisar status --json with HISAR_NO_LAUNCH=1 and the rest of the environment", async () => {
    const { run, calls } = answering({ code: 0, stdout: '{"state":"locked"}' });
    await probeStatus(
      "/Applications/Hisar.app/Contents/Helpers/hisar",
      { HOME: "/Users/a", HISAR_NO_LAUNCH: "0" },
      run,
    );
    expect(calls).toEqual([
      {
        argv: ["/Applications/Hisar.app/Contents/Helpers/hisar", "status", "--json"],
        environment: { HOME: "/Users/a", HISAR_NO_LAUNCH: "1" },
      },
    ]);
  });

  test("reads the state from a real process", async () => {
    const directory = mkdtempSync(join(tmpdir(), "hisar-omp-status-"));
    try {
      const fakeHisar = join(directory, "hisar");
      writeFileSync(
        fakeHisar,
        '#!/bin/sh\n[ "$HISAR_NO_LAUNCH" = 1 ] && [ "$1 $2" = "status --json" ] && echo \'{"mode":"app","state":"unlocked","account":"a@example.com"}\'\n',
      );
      chmodSync(fakeHisar, 0o755);
      expect(await probeStatus(fakeHisar, { PATH: "/usr/bin:/bin" })).toBe("unlocked");
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });
});
