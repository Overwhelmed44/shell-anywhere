// Keep scrolling between native dragover events, including when the pointer rests
// at an edge. Report actual movement so the drop target can follow the new rows.
export function createFileDragScroll(
  element: HTMLElement,
  onScroll: (x: number, y: number) => void,
) {
  let frame: number | null = null;
  let previousTime: number | null = null;
  let point: { x: number; y: number } | null = null;
  let wheelUntil = 0;

  const pause = () => {
    if (frame !== null) window.cancelAnimationFrame(frame);
    frame = null;
    previousTime = null;
  };

  const stop = () => {
    pause();
    point = null;
    wheelUntil = 0;
  };

  const inside = () => {
    if (!point || !element.isConnected) return false;
    const bounds = element.getBoundingClientRect();
    return bounds.height > 0 && point.x >= bounds.left && point.x < bounds.right &&
      point.y >= bounds.top && point.y < bounds.bottom;
  };

  const velocity = () => {
    if (!point || !inside() || Date.now() < wheelUntil) return 0;
    const bounds = element.getBoundingClientRect();

    const edge = Math.min(72, bounds.height / 3);
    const top = Math.max(0, (bounds.top + edge - point.y) / edge);
    const bottom = Math.max(0, (point.y - bounds.bottom + edge) / edge);
    const maximum = element.scrollHeight - element.clientHeight;
    if (top > 0 && element.scrollTop > 0) return -(80 + 640 * top * top);
    if (bottom > 0 && element.scrollTop < maximum) return 80 + 640 * bottom * bottom;
    return 0;
  };

  const tick = (time: number) => {
    frame = null;
    const speed = velocity();
    if (!speed || !point) {
      pause();
      return;
    }
    const elapsed = previousTime === null ? 1000 / 60 : Math.min(40, time - previousTime);
    previousTime = time;
    const before = element.scrollTop;
    element.scrollTop += speed * elapsed / 1000;
    if (element.scrollTop === before) {
      pause();
      return;
    }
    onScroll(point.x, point.y);
    if (point) frame = window.requestAnimationFrame(tick);
  };

  return {
    update(x: number, y: number) {
      point = { x, y };
      if (!inside()) { stop(); return; }
      if (!velocity()) {
        pause();
        return;
      }
      if (frame === null) frame = window.requestAnimationFrame(tick);
    },
    wheel(deltaX: number, deltaY: number, mode: number) {
      if (!point || !inside()) return false;
      pause();
      wheelUntil = Date.now() + 180;
      const factorY = mode === 1 ? 16 : mode === 2 ? element.clientHeight : 1;
      const factorX = mode === 1 ? 16 : mode === 2 ? element.clientWidth : 1;
      const beforeTop = element.scrollTop;
      const beforeLeft = element.scrollLeft;
      element.scrollTop += deltaY * factorY;
      element.scrollLeft += deltaX * factorX;
      if (element.scrollTop !== beforeTop || element.scrollLeft !== beforeLeft) onScroll(point.x, point.y);
      return true;
    },
    stop,
  };
}
