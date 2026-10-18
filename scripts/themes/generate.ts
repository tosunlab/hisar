import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { renderThemeCSS, renderThemeKotlin, renderThemeSwift, type ThemeSource } from "./render.ts";

const repositoryDirectory = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const source: ThemeSource = JSON.parse(readFileSync(join(repositoryDirectory, "assets/themes/themes.json"), "utf8"));

const themeCSS = renderThemeCSS(source);
writeFileSync(join(repositoryDirectory, "desktop/frontend/src/themes.css"), themeCSS);
writeFileSync(join(repositoryDirectory, "admin/src/themes.css"), themeCSS);
writeFileSync(join(repositoryDirectory, "mobile/ios/Shared/Views/ThemePalettes.swift"), renderThemeSwift(source));
writeFileSync(join(repositoryDirectory, "mobile/android/model/src/main/kotlin/com/tryhisar/hisar/model/theme/ThemePalettes.kt"), renderThemeKotlin(source));
