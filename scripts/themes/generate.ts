import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { renderThemeCSS, renderThemeSwift, type ThemeSource } from "./render.ts";

const repositoryDirectory = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const source: ThemeSource = JSON.parse(readFileSync(join(repositoryDirectory, "assets/themes/themes.json"), "utf8"));

writeFileSync(join(repositoryDirectory, "desktop/frontend/src/themes.css"), renderThemeCSS(source));
writeFileSync(join(repositoryDirectory, "mobile/ios/Shared/Views/ThemePalettes.swift"), renderThemeSwift(source));
