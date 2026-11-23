type Point = { at: number; value: number | null };

// Keep gaps and elapsed time intact instead of joining unknown counter values.
export function metricChartPath(points: Point[], maximum: number): string {
  const timestamps = points.map((point) => point.at).filter(Number.isFinite);
  if (timestamps.length < 2 || !Number.isFinite(maximum) || maximum <= 0) return "";
  const start = timestamps[0];
  const duration = timestamps[timestamps.length - 1] - start;
  if (duration <= 0) return "";
  let drawing = false;
  return points.map(({ at, value }) => {
    if (value === null || !Number.isFinite(value) || !Number.isFinite(at)) {
      drawing = false;
      return "";
    }
    const x = Math.max(0, Math.min(1, (at - start) / duration)) * 480;
    const y = 100 - Math.max(0, Math.min(1, value / maximum)) * 100;
    const command = `${drawing ? "L" : "M"}${x.toFixed(2)} ${y.toFixed(2)}`;
    drawing = true;
    return command;
  }).join(" ");
}
