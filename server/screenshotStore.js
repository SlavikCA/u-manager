const SLOTS = {
  min10:  { target: 10 * 60 * 1000, maxAge: 15 * 60 * 1000 },
  min30:  { target: 30 * 60 * 1000, maxAge: 45 * 60 * 1000 },
  min60:  { target: 60 * 60 * 1000, maxAge: 90 * 60 * 1000 },
  min120: { target: 120 * 60 * 1000, maxAge: 180 * 60 * 1000 },
};

const SLOT_NAMES = ['recent', 'min10', 'min30', 'min60', 'min120'];

// Map<computerId, { recent, min10, min30, min60, min120 }>
// Each slot: { buffer, timestamp } | null
const screenshots = new Map();

function getEntry(computerId) {
  if (!screenshots.has(computerId)) {
    screenshots.set(computerId, {
      recent: null,
      min10: null,
      min30: null,
      min60: null,
      min120: null,
    });
  }
  return screenshots.get(computerId);
}

module.exports = {
  set(computerId, buffer) {
    const entry = getEntry(computerId);
    const now = Date.now();

    // Before overwriting recent, try to promote it into an aged slot
    if (entry.recent) {
      const age = now - entry.recent.timestamp;

      for (const [slotName, { target, maxAge }] of Object.entries(SLOTS)) {
        if (age > maxAge) continue; // too old for this slot
        if (age < target * 0.5) continue; // too young for this slot

        const current = entry[slotName];
        if (!current) {
          // Slot empty — fill it
          entry[slotName] = entry.recent;
        } else {
          // Replace if new candidate is closer to the target age
          const currentDist = Math.abs((now - current.timestamp) - target);
          const candidateDist = Math.abs(age - target);
          if (candidateDist < currentDist) {
            entry[slotName] = entry.recent;
          }
        }
      }
    }

    // Set new recent
    entry.recent = { buffer, timestamp: now };

    // Expire stale aged slots
    for (const [slotName, { maxAge }] of Object.entries(SLOTS)) {
      if (entry[slotName] && (now - entry[slotName].timestamp) > maxAge) {
        entry[slotName] = null;
      }
    }
  },

  get(computerId, slot) {
    if (!slot || !SLOT_NAMES.includes(slot)) slot = 'recent';
    const entry = screenshots.get(computerId);
    if (!entry || !entry[slot]) return null;
    return entry[slot].buffer;
  },

  getSlotInfo(computerId) {
    const entry = screenshots.get(computerId);
    if (!entry) return null;
    const info = {};
    for (const name of SLOT_NAMES) {
      info[name] = entry[name] ? { timestamp: entry[name].timestamp } : null;
    }
    return info;
  },

  remove(computerId) {
    screenshots.delete(computerId);
  }
};
