const express = require('express');
const router = express.Router();
const Token = require('../models/Token');
const AuditLog = require('../models/AuditLog');

// GET /tokens - Show token management page
router.get('/', (req, res) => {
  const tokens = Token.findAll();
  
  res.render('tokens.html', {
    tokens,
    title: 'Agent Tokens',
    newToken: null
  });
});

// POST /tokens/generate - Generate a new token
router.post('/generate', (req, res) => {
  const { name } = req.body;
  
  const token = Token.generate(name || null);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.GENERATE_TOKEN,
    null,
    null,
    `Token: ${token.token.substring(0, 8)}...`
  );
  
  const tokens = Token.findAll();
  
  // For HTMX request, return the tokens list partial with new token highlighted
  if (req.headers['hx-request']) {
    return res.render('partials/tokens-list.html', { 
      tokens,
      newToken: token
    });
  }
  
  res.render('tokens.html', {
    tokens,
    title: 'Agent Tokens',
    newToken: token
  });
});

// DELETE /tokens/:id - Revoke a token
router.delete('/:id', (req, res) => {
  const token = Token.findById(req.params.id);
  
  if (!token) {
    if (req.headers['hx-request']) {
      return res.status(404).send('Token not found');
    }
    return res.status(404).render('error.html', {
      message: 'Token not found',
      status: 404
    });
  }
  
  Token.revoke(req.params.id);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.REVOKE_TOKEN,
    null,
    null,
    `Token: ${token.token.substring(0, 8)}...`
  );
  
  // For HTMX request, return updated tokens list
  if (req.headers['hx-request']) {
    const tokens = Token.findAll();
    return res.render('partials/tokens-list.html', { 
      tokens,
      newToken: null
    });
  }
  
  res.redirect('/tokens');
});

// POST /tokens/:id/revoke - Alternative revoke endpoint for forms
router.post('/:id/revoke', (req, res) => {
  const token = Token.findById(req.params.id);
  
  if (!token) {
    if (req.headers['hx-request']) {
      return res.status(404).send('Token not found');
    }
    return res.status(404).render('error.html', {
      message: 'Token not found',
      status: 404
    });
  }
  
  Token.revoke(req.params.id);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.REVOKE_TOKEN,
    null,
    null,
    `Token: ${token.token.substring(0, 8)}...`
  );
  
  // For HTMX request, return updated tokens list
  if (req.headers['hx-request']) {
    const tokens = Token.findAll();
    return res.render('partials/tokens-list.html', { 
      tokens,
      newToken: null
    });
  }
  
  res.redirect('/tokens');
});

module.exports = router;
