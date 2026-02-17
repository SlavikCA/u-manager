// Keep screenshots for up to 2 hours to cover 4 quarter-hour slots
const MAX_HISTORY_AGE = 2 * 60 * 60 * 1000;

// Map<computerId, { buffer, timestamp }[]>
const screenshots = new Map();

function getQuarterTargets(now) {
  // Find the most recent quarter-hour boundary, then go back 4 quarters
  const d = new Date(now);
  const min = d.getMinutes();
  const quarterMin = Math.floor(min / 15) * 15;
  d.setMinutes(quarterMin, 0, 0);

  const targets = [];
  for (let i = 1; i <= 4; i++) {
    targets.push(d.getTime() - (i - 1) * 15 * 60 * 1000);
  }
  return targets; // [most recent quarter, ..., oldest quarter]
}

module.exports = {
  set(computerId, buffer) {
    if (!screenshots.has(computerId)) screenshots.set(computerId, []);
    const history = screenshots.get(computerId);
    const now = Date.now();

    history.push({ buffer, timestamp: now });

    // Prune entries older than MAX_HISTORY_AGE
    const cutoff = now - MAX_HISTORY_AGE;
    while (history.length > 0 && history[0].timestamp < cutoff) {
      history.shift();
    }
  },

  getRecent(computerId) {
    const history = screenshots.get(computerId);
    if (!history || history.length === 0) return null;
    return history[history.length - 1].buffer;
  },

  getSlots(computerId) {
    const history = screenshots.get(computerId);
    if (!history || history.length === 0) return [];

    const now = Date.now();
    const targets = getQuarterTargets(now);
    const slots = [];

    for (const target of targets) {
      let best = null;
      let bestDist = Infinity;

      for (const entry of history) {
        const dist = Math.abs(entry.timestamp - target);
        // Only consider screenshots within 7.5 min of the target
        if (dist < bestDist && dist <= 7.5 * 60 * 1000) {
          best = entry;
          bestDist = dist;
        }
      }

      const minsAgo = Math.round((now - target) / 60000);
      const label = minsAgo < 60
        ? minsAgo + ' min ago'
        : Math.floor(minsAgo / 60) + 'h ' + (minsAgo % 60) + 'm ago';

      slots.push({ label, entry: best });
    }

    return slots;
  },

  remove(computerId) {
    screenshots.delete(computerId);
  }
};
