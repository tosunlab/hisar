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

const baseNote =
  "Hisar holds the user's secrets. Give one to a command with `NAME=hisar://<vault>/<item>/<field> hisar run -- <command>`, never by printing it. Read the `hisar` skill before using Hisar.";

// describeNote is the note every prompt carries; omp's PTY shell keeps the PATH omp started with, so it names
// hisar by its full path when that PATH lacks it.
export function describeNote(location: HisarLocation): string {
  return location.isOnInheritedPath ? baseNote : `${baseNote} Call hisar as \`${location.path}\`.`;
}
