export interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface Viewport {
  width: number;
  height: number;
}

export const iconSize = 20;
const iconInset = 8;
const menuGap = 4;
const viewportMargin = 8;
const minimumMenuWidth = 280;
const maximumMenuWidth = 360;

export function isFieldLargeEnough(rect: Rect): boolean {
  return rect.width >= iconSize * 3 && rect.height >= 16;
}

export function computeUnionRect(allRect: Rect[]): Rect {
  const left = Math.min(...allRect.map((rect) => rect.left));
  const top = Math.min(...allRect.map((rect) => rect.top));
  const right = Math.max(...allRect.map((rect) => rect.left + rect.width));
  const bottom = Math.max(...allRect.map((rect) => rect.top + rect.height));
  return { left, top, width: right - left, height: bottom - top };
}

export function isRectInViewport(rect: Rect, viewport: Viewport): boolean {
  return rect.top + rect.height > 0 && rect.top < viewport.height && rect.left + rect.width > 0 && rect.left < viewport.width;
}

export function computeIconRect(fieldRect: Rect): Rect {
  return {
    left: fieldRect.left + fieldRect.width - iconSize - iconInset,
    top: fieldRect.top + (fieldRect.height - iconSize) / 2,
    width: iconSize,
    height: iconSize,
  };
}

// computeIconRectBeside keeps the icon out of split code boxes, which are too narrow to hold it.
export function computeIconRectBeside(fieldRect: Rect): Rect {
  return { left: fieldRect.left + fieldRect.width + menuGap * 2, top: fieldRect.top + (fieldRect.height - iconSize) / 2, width: iconSize, height: iconSize };
}

// computeMenuRect puts the menu below the field, or above it when only that fits. When neither fits, it takes the side with
// more room and is cut to it, scrolling inside: a menu that runs past the window is not fully visible, and the visibility
// guards would ignore every click on it.
export function computeMenuRect(fieldRect: Rect, menuHeight: number, viewport: Viewport): Rect {
  const width = Math.min(Math.max(fieldRect.width, minimumMenuWidth), maximumMenuWidth, viewport.width - viewportMargin * 2);
  const left = Math.min(Math.max(fieldRect.left, viewportMargin), viewport.width - width - viewportMargin);
  const below = fieldRect.top + fieldRect.height + menuGap;
  const spaceBelow = viewport.height - viewportMargin - below;
  const spaceAbove = fieldRect.top - menuGap - viewportMargin;
  if (menuHeight <= spaceBelow) return { left, top: below, width, height: menuHeight };
  if (menuHeight <= spaceAbove) return { left, top: fieldRect.top - menuGap - menuHeight, width, height: menuHeight };
  if (spaceBelow >= spaceAbove) return { left, top: below, width, height: Math.max(spaceBelow, 0) };
  const height = Math.max(spaceAbove, 0);
  return { left, top: fieldRect.top - menuGap - height, width, height };
}
