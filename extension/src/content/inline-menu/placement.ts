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

export function computeMenuRect(fieldRect: Rect, menuHeight: number, viewport: Viewport): Rect {
  const width = Math.min(Math.max(fieldRect.width, minimumMenuWidth), maximumMenuWidth, viewport.width - viewportMargin * 2);
  const left = Math.min(Math.max(fieldRect.left, viewportMargin), viewport.width - width - viewportMargin);
  const below = fieldRect.top + fieldRect.height + menuGap;
  const above = fieldRect.top - menuGap - menuHeight;
  const fitsBelow = below + menuHeight <= viewport.height - viewportMargin;
  const top = fitsBelow || above < viewportMargin ? below : above;
  return { left, top, width, height: menuHeight };
}
