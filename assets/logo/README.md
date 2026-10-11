# Hisar logo

| File | Use |
|---|---|
| `hisar-mark.svg` | The mark: light rounded square with the fortress tower. Source of truth |
| `hisar-tower.svg` | The tower glyph alone (used as the macOS Icon Composer layer) |
| `hisar-app-icon.svg` | The mark on Apple's icon grid (824 px square on a 1024 px canvas), source of `desktop/build/appicon.png` |
| `hisar-ios-icon.svg` | The mark full-bleed and opaque (iOS rounds the corners), source of the iPhone app icon `mobile/ios/App/Assets.xcassets/AppIcon.appiconset/AppIcon-1024.png` |
| `hisar-menu-bar.svg` | The tower in black on a square canvas, source of the macOS menu bar template image `desktop/build/menu-bar-icon.png` (44 px, shown at 22 points); Linux trays use `desktop/build/tray-icon-linux.png`, the mark at 64 px |
| `hisar-wordmark-on-dark.svg` / `-on-light.svg` | Mark and "Hisar" side by side, for dark and light backgrounds |
| `png/` | Rendered PNGs: the mark at 1024–16 px, the wordmarks at 1008 and 504 px wide (transparent background) |

Colors: mark background `#f2f3f5`, tower `#0b0c0e`. The in-app logo (`desktop/frontend/src/components/HisarLogo.vue`) uses the same path.

## Regenerating

```sh
scripts/render-logo.sh    # PNGs, desktop/build/appicon.png, the iPhone app icon and the DMG background, via headless Chrome on macOS
```

The macOS app icon comes from `desktop/build/appicon.icon` (Icon Composer, tower layer on a `#f2f3f5` fill). The build's change detection does not notice edits inside that folder, so after changing it run from `desktop/build`:

```sh
wails3 generate icons -input appicon.png -macfilename darwin/icons.icns -windowsfilename windows/icon.ico -iconcomposerinput appicon.icon -macassetdir darwin
```
