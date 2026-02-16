const screenshots = new Map();

module.exports = {
  set(computerId, buffer) {
    screenshots.set(computerId, buffer);
  },

  get(computerId) {
    return screenshots.get(computerId) || null;
  },

  remove(computerId) {
    screenshots.delete(computerId);
  }
};
