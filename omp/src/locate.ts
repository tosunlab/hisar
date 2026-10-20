import { accessSync, constants, statSync } from "node:fs";
import { delimiter, dirname, join } from "node:path";

export interface HisarLocation {
  path: string;
  isOnInheritedPath: boolean;
}

const bundledHelper = join("Hisar.app", "Contents", "Helpers", "hisar");

export function isExecutableFile(path: string): boolean {
  try {
    accessSync(path, constants.X_OK);
    return statSync(path).isFile();
  } catch {
    return false;
  }
}

// findHisar looks on PATH, then in the Hisar.app in /Applications, then in the one in ~/Applications.
export function findHisar(
  environment: Record<string, string | undefined>,
  isExecutable: (path: string) => boolean = isExecutableFile,
): HisarLocation | undefined {
  for (const directory of (environment.PATH ?? "").split(delimiter)) {
    if (directory === "") continue;
    const path = join(directory, "hisar");
    if (isExecutable(path)) return { path, isOnInheritedPath: true };
  }
  const appCandidates = [join("/Applications", bundledHelper)];
  if (environment.HOME) appCandidates.push(join(environment.HOME, "Applications", bundledHelper));
  const path = appCandidates.find(isExecutable);
  return path === undefined ? undefined : { path, isOnInheritedPath: false };
}

// prependToPath puts hisar's folder first on PATH, once.
export function prependToPath(pathVariable: string | undefined, location: HisarLocation): string | undefined {
  if (location.isOnInheritedPath) return pathVariable;
  const directory = dirname(location.path);
  const entries = (pathVariable ?? "").split(delimiter).filter((entry) => entry !== "");
  if (entries.includes(directory)) return pathVariable;
  return [directory, ...entries].join(delimiter);
}

// describePathNote names hisar's full path when omp's PTY shell, which keeps the PATH omp started with, lacks it.
// The hisar MCP server's instructions carry the rest of the guidance into the prompt.
export function describePathNote(location: HisarLocation): string | undefined {
  if (location.isOnInheritedPath) return undefined;
  return `Hisar's command-line tool is not on the shell's PATH; call hisar as \`${location.path}\`.`;
}
