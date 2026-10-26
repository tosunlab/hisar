import { copyField } from "./store";

// bindCopyRow makes a field row on item detail a button that copies its field, to spread on the row with v-bind: a click
// anywhere on it, or Enter or Space while the row has focus. A click on a control inside the row, such as reveal or open,
// is that control's alone, and a click that ends a text selection in the row leaves the selection to copy by hand.
export function bindCopyRow(name: string, copy: () => unknown): Record<string, unknown> {
  return {
    role: "button",
    tabindex: 0,
    "aria-label": `Copy ${name}`,
    "data-copy-row": "",
    onClick: (event: MouseEvent) => {
      if (isRowClick(event)) {
        copy();
      }
    },
    onKeydown: (event: KeyboardEvent) => {
      if (event.target === event.currentTarget && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault();
        copy();
      }
    },
  };
}

// bindFieldRow is bindCopyRow for a field the backend copies by name, as the row's copy button does.
export function bindFieldRow(fieldName: string, label: string): Record<string, unknown> {
  return bindCopyRow(fieldName, () => copyField(fieldName, label));
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
