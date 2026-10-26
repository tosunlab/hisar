import { copyField } from "./store";

export function bindCopyTarget(name: string): Record<string, unknown> {
  return { role: "button", tabindex: 0, "aria-label": `Copy ${name}`, "data-copy-target": "" };
}

export function bindCopyRow(copy: () => unknown): Record<string, unknown> {
  return {
    "data-copy-row": "",
    onClick: (event: MouseEvent) => {
      if (isRowClick(event)) {
        copy();
      }
    },
    onKeydown: (event: KeyboardEvent) => {
      if (event.target === (event.currentTarget as HTMLElement).querySelector("[data-copy-target]") && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault();
        copy();
      }
    },
  };
}

// bindFieldRow is bindCopyRow for a field the backend copies by name, as the row's copy button does.
export function bindFieldRow(fieldName: string, label: string): Record<string, unknown> {
  return bindCopyRow(() => copyField(fieldName, label));
}

function isRowClick(event: MouseEvent): boolean {
  const row = event.currentTarget as HTMLElement;
  const control = (event.target as Element).closest("button, a, input, select, textarea");
  if (control !== null && row.contains(control)) {
    return false;
  }
  const selection = window.getSelection();
  return !selection || selection.isCollapsed || !(row.contains(selection.anchorNode) || row.contains(selection.focusNode));
}
