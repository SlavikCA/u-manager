const express = require('express');
const router = express.Router();
const Computer = require('../models/Computer');

// GET / - Dashboard showing all computers
router.get('/', (req, res) => {
  // Mark stale computers as offline (not seen in 30 seconds)
  Computer.markStaleComputersOffline(30);
  
  const computers = Computer.findAll();
  
  res.render('index.html', { 
    computers,
    title: 'Dashboard'
  });
});

// GET /computers-list - HTMX partial for refreshing computer list
router.get('/computers-list', (req, res) => {
  // Mark stale computers as offline
  Computer.markStaleComputersOffline(30);
  
  const computers = Computer.findAll();
  
  res.render('partials/computers-list.html', { computers });
});

module.exports = router;
