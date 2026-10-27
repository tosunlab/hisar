// The main window's resizable columns, in CSS pixels. The detail column takes the rest and never gets narrower than
// detailMinWidth: when the window narrows, the sidebar and the list give way down to their minimums first.
export const allColumnBound = {
  sidebar: { initial: 228, min: 180, max: 360 },
  list: { initial: 300, min: 240, max: 480 },
} as const;
export const detailMinWidth = 400;

export type ColumnID = keyof typeof allColumnBound;
export type ColumnWidths = Record<ColumnID, number>;

const columnWidthsStorageKey = "hisar.mainWindow.columnWidths";

export function clampColumnWidth(columnID: ColumnID, width: number): number {
  const bound = allColumnBound[columnID];
  return Math.round(Math.min(bound.max, Math.max(bound.min, width)));
}

// loadColumnWidths reads this device's widths; a missing or malformed value falls back to the default.
export function loadColumnWidths(): ColumnWidths {
  let stored: Partial<Record<ColumnID, unknown>> | null = null;
  try {
    stored = JSON.parse(window.localStorage.getItem(columnWidthsStorageKey) ?? "null");
  } catch {
    stored = null;
  }
  const read = (columnID: ColumnID): number => {
    const width = stored?.[columnID];
    return typeof width === "number" && Number.isFinite(width) ? clampColumnWidth(columnID, width) : allColumnBound[columnID].initial;
  };
  return { sidebar: read("sidebar"), list: read("list") };
}

export function saveColumnWidths(widths: ColumnWidths): void {
  try {
    window.localStorage.setItem(columnWidthsStorageKey, JSON.stringify(widths));
  } catch {
    // Storage can be unavailable; the widths then last until the window reloads.
  }
}
