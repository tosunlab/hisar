// readAncestorOrigins lists the origins of the frames above window, the parent first, as location.ancestorOrigins does in
// Chrome. Firefox has no ancestorOrigins, so there they are read from each parent frame. A parent of another origin cannot
// be read from this frame and counts as "null", which never equals a web origin: the frame rule fails closed instead of
// passing for a frame inside another site.
export function readAncestorOrigins(window: Window): string[] {
  if (window.location.ancestorOrigins) return Array.from(window.location.ancestorOrigins);
  const allAncestorOrigin: string[] = [];
  for (let frame = window; frame.parent && frame !== frame.top; ) {
    frame = frame.parent;
    try {
      allAncestorOrigin.push(frame.location.origin);
    } catch {
      allAncestorOrigin.push("null");
    }
  }
  return allAncestorOrigin;
}

// readFrameLocation is the frame's address and its ancestors' origins, for the background's frame choice. It runs inside
// each frame through scripting.executeScript, which sends only this function's source, so it repeats readAncestorOrigins
// instead of calling it.
export function readFrameLocation(): { url: string; origin: string; allAncestorOrigin: string[] } {
  const allAncestorOrigin: string[] = [];
  if (location.ancestorOrigins) {
    allAncestorOrigin.push(...Array.from(location.ancestorOrigins));
  } else {
    for (let frame: Window = window; frame !== frame.top; ) {
      frame = frame.parent;
      try {
        allAncestorOrigin.push(frame.location.origin);
      } catch {
        allAncestorOrigin.push("null");
      }
    }
  }
  return { url: location.href, origin: location.origin, allAncestorOrigin };
}
