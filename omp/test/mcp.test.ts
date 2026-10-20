import { afterEach, describe, expect, test } from "bun:test";
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const pluginSource = resolve(import.meta.dir, "..");

interface ServerConfig {
  command: string;
  args: string[];
  timeout?: number;
}

const { mcpServers } = (await Bun.file(join(pluginSource, "mcp.json")).json()) as {
  mcpServers: Record<string, ServerConfig>;
};
const server = mcpServers.hisar!;

let scratch: string;

afterEach(() => rmSync(scratch, { recursive: true, force: true }));

function createScratch(): string {
  scratch = realpathSync(mkdtempSync(join(tmpdir(), "hisar-omp-mcp-")));
  return scratch;
}

function installPlugin(root: string) {
  mkdirSync(join(root, "bin"), { recursive: true });
  copyFileSync(join(pluginSource, "mcp.json"), join(root, "mcp.json"));
  copyFileSync(join(pluginSource, "bin", "run-hisar"), join(root, "bin", "run-hisar"));
  chmodSync(join(root, "bin", "run-hisar"), 0o755);
}

// installFakeHisar writes a hisar that prints its name and arguments, one a line, and exits with 5.
function installFakeHisar(path: string, name: string) {
  mkdirSync(resolve(path, ".."), { recursive: true });
  writeFileSync(path, `#!/bin/sh\nprintf '%s\\n' ${name} "$@"\nexit 5\n`);
  chmodSync(path, 0o755);
}

// startServer starts the server as omp does: a relative command is rooted at the folder that holds mcp.json.
function startServer(pluginRoot: string, pathDirectories: string[]) {
  const command = server.command.startsWith("./") ? join(pluginRoot, server.command) : server.command;
  const child = Bun.spawnSync([command, ...server.args, "two words"], {
    cwd: scratch,
    env: { PATH: [...pathDirectories, "/usr/bin", "/bin"].join(":"), HOME: join(scratch, "home") },
  });
  return { code: child.exitCode, stdout: child.stdout.toString().split("\n").filter(Boolean) };
}

describe("the hisar MCP server", () => {
  test("runs hisar mcp --no-reveal with no client timeout", () => {
    const root = createScratch();
    installPlugin(join(root, "omp"));
    installFakeHisar(join(root, "path", "hisar"), "path");
    expect(startServer(join(root, "omp"), [join(root, "path")])).toEqual({
      code: 5,
      stdout: ["path", "mcp", "--no-reveal", "two words"],
    });
    expect(server.timeout).toBe(0);
  });

  test("inside Hisar.app, runs the app's own hisar before one on PATH", () => {
    const root = createScratch();
    const plugin = join(root, "Hisar.app", "Contents", "Resources", "omp-plugin");
    installPlugin(plugin);
    installFakeHisar(join(root, "Hisar.app", "Contents", "Helpers", "hisar"), "bundled");
    installFakeHisar(join(root, "path", "hisar"), "path");
    expect(startServer(plugin, [join(root, "path")]).stdout[0]).toBe("bundled");
  });

  test("finds the app's hisar when omp reaches the plugin through a link", () => {
    const root = createScratch();
    const plugin = join(root, "Hisar.app", "Contents", "Resources", "omp-plugin");
    installPlugin(plugin);
    installFakeHisar(join(root, "Hisar.app", "Contents", "Helpers", "hisar"), "bundled");
    symlinkSync(plugin, join(root, "linked"));
    expect(startServer(join(root, "linked"), []).stdout[0]).toBe("bundled");
  });
});
