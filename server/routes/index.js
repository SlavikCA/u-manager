const express = require('express');
const router = express.Router();
const Computer = require('../models/Computer');

function formatOfflineDuration(lastSeenAt) {
  if (!lastSeenAt) return 'Offline';
  const diffMs = Date.now() - new Date(lastSeenAt + 'Z').getTime();
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return 'Offline for ' + diffSec + 's';
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return 'Offline for ' + diffMin + 'min';
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return 'Offline for ' + diffHr + 'h';
  const diffDays = Math.floor(diffHr / 24);
  return 'Offline for ' + diffDays + 'd';
}

function enrichComputers(computers) {
  return computers.map(c => ({
    ...c,
    status_label: c.status === 'online' ? 'Online' : formatOfflineDuration(c.last_seen_at)
  }));
}

// GET / - Dashboard showing all computers
router.get('/', (req, res) => {
  Computer.markStaleComputersOffline(22);

  const computers = enrichComputers(Computer.findAll());

  res.render('index.html', {
    computers,
    title: 'Dashboard'
  });
});

// GET /computers-list - HTMX partial for refreshing computer list
router.get('/computers-list', (req, res) => {
  Computer.markStaleComputersOffline(22);

  const computers = enrichComputers(Computer.findAll());

  res.render('partials/computers-list.html', { computers });
});

module.exports = router;
