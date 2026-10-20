import { describe, expect, test } from "bun:test";
import { describeNote, findHisar, prependToPath } from "../src/locate";

const appHelper = "/Applications/Hisar.app/Contents/Helpers/hisar";
const homeHelper = "/Users/a/Applications/Hisar.app/Contents/Helpers/hisar";

function existing(...paths: string[]) {
  return (path: string) => paths.includes(path);
}

describe("findHisar", () => {
  const environment = { PATH: "/usr/bin:/opt/homebrew/bin", HOME: "/Users/a" };

  test("finds hisar on PATH first, in PATH order", () => {
    expect(findHisar(environment, existing("/opt/homebrew/bin/hisar", "/usr/bin/hisar", appHelper))).toEqual({
      path: "/usr/bin/hisar",
      isOnInheritedPath: true,
    });
  });

  test("then in /Applications, then in ~/Applications", () => {
    expect(findHisar(environment, existing(appHelper, homeHelper))).toEqual({
      path: appHelper,
      isOnInheritedPath: false,
    });
    expect(findHisar(environment, existing(homeHelper))).toEqual({ path: homeHelper, isOnInheritedPath: false });
  });

  test("finds nothing when Hisar is not installed", () => {
    expect(findHisar(environment, existing())).toBeUndefined();
    expect(findHisar({}, existing(homeHelper))).toBeUndefined();
  });
});

describe("prependToPath", () => {
  test("puts the app's helper folder first, once", () => {
    const location = { path: appHelper, isOnInheritedPath: false };
    const path = prependToPath("/usr/bin:/bin", location);
    expect(path).toBe("/Applications/Hisar.app/Contents/Helpers:/usr/bin:/bin");
    expect(prependToPath(path, location)).toBe(path);
    expect(prependToPath(undefined, location)).toBe("/Applications/Hisar.app/Contents/Helpers");
  });

  test("leaves PATH alone when hisar is already on it", () => {
    expect(prependToPath("/usr/bin", { path: "/usr/bin/hisar", isOnInheritedPath: true })).toBe("/usr/bin");
  });
});

describe("describeNote", () => {
  test("names hisar run, hisar:// references and the hisar skill", () => {
    const note = describeNote({ path: "/usr/bin/hisar", isOnInheritedPath: true });
    expect(note).toContain("hisar run --");
    expect(note).toContain("hisar://");
    expect(note).toContain("`hisar` skill");
    expect(note).not.toContain("/usr/bin/hisar");
  });

  test("carries the absolute path when hisar is not on the inherited PATH", () => {
    expect(describeNote({ path: homeHelper, isOnInheritedPath: false })).toEndWith(`Call hisar as \`${homeHelper}\`.`);
  });

  // About 4 characters a token in English; the note with a long path stays well under 80 tokens.
  test("stays under 80 tokens", () => {
    const note = describeNote({ path: homeHelper, isOnInheritedPath: false });
    expect(note.length / 4).toBeLessThan(80);
    expect(note.split(/\s+/).length).toBeLessThan(55);
  });
});
