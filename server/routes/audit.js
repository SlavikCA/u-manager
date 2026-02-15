const express = require('express');
const router = express.Router();
const AuditLog = require('../models/AuditLog');

// GET /audit - Show audit log page
router.get('/', (req, res) => {
  const page = parseInt(req.query.page) || 1;
  const limit = 50;
  const offset = (page - 1) * limit;
  
  const logs = AuditLog.findAll(limit, offset);
  const totalCount = AuditLog.getCount();
  const totalPages = Math.ceil(totalCount / limit);
  
  res.render('audit.html', {
    logs,
    title: 'Audit Log',
    currentPage: page,
    totalPages,
    totalCount
  });
});

// GET /audit/list - HTMX partial for refreshing audit log
router.get('/list', (req, res) => {
  const page = parseInt(req.query.page) || 1;
  const limit = 50;
  const offset = (page - 1) * limit;
  
  const logs = AuditLog.findAll(limit, offset);
  const totalCount = AuditLog.getCount();
  const totalPages = Math.ceil(totalCount / limit);
  
  res.render('partials/audit-list.html', {
    logs,
    currentPage: page,
    totalPages,
    totalCount
  });
});

module.exports = router;
