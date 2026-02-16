const express = require('express');
const router = express.Router();
const Computer = require('../models/Computer');
const User = require('../models/User');
const Command = require('../models/Command');
const AuditLog = require('../models/AuditLog');
const screenshotStore = require('../screenshotStore');

// GET /computers/:id - Show computer management page
router.get('/:id', (req, res) => {
  const computer = Computer.getWithUsers(req.params.id);
  
  if (!computer) {
    return res.status(404).render('error.html', {
      message: 'Computer not found',
      status: 404
    });
  }
  
  const pendingCommands = Command.findPendingByComputerId(computer.id);
  
  res.render('computer.html', {
    computer,
    pendingCommands,
    title: computer.hostname
  });
});

// GET /computers/:id/screenshot - Serve screenshot by slot
router.get('/:id/screenshot', (req, res) => {
  const slot = req.query.slot || 'recent';
  const data = screenshotStore.get(parseInt(req.params.id), slot);
  if (!data) {
    return res.status(404).send('No screenshot available');
  }
  res.set('Content-Type', 'image/jpeg');
  res.set('Cache-Control', 'no-cache');
  res.send(data);
});

// GET /computers/:id/users - HTMX partial for refreshing users list
router.get('/:id/users', (req, res) => {
  const computer = Computer.getWithUsers(req.params.id);
  
  if (!computer) {
    return res.status(404).send('Computer not found');
  }
  
  const pendingCommands = Command.findPendingByComputerId(computer.id);
  
  res.render('partials/users-list.html', { 
    computer,
    pendingCommands
  });
});

// POST /computers/:id/user/:username/disable - Disable a user
router.post('/:id/user/:username/disable', (req, res) => {
  const computer = Computer.findById(req.params.id);
  
  if (!computer) {
    if (req.headers['hx-request']) {
      return res.status(404).send('Computer not found');
    }
    return res.status(404).render('error.html', {
      message: 'Computer not found',
      status: 404
    });
  }
  
  const user = User.findByComputerAndUsername(computer.id, req.params.username);
  
  if (!user) {
    if (req.headers['hx-request']) {
      return res.status(404).send('User not found');
    }
    return res.status(404).render('error.html', {
      message: 'User not found',
      status: 404
    });
  }
  
  // Create disable command
  Command.create(computer.id, Command.TYPES.DISABLE_USER, user.username);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.DISABLE_USER,
    computer.hostname,
    user.username
  );
  
  // Return updated users list for HTMX
  if (req.headers['hx-request']) {
    const updatedComputer = Computer.getWithUsers(computer.id);
    const pendingCommands = Command.findPendingByComputerId(computer.id);
    return res.render('partials/users-list.html', { 
      computer: updatedComputer,
      pendingCommands
    });
  }
  
  res.redirect(`/computers/${computer.id}`);
});

// POST /computers/:id/user/:username/enable - Enable a user
router.post('/:id/user/:username/enable', (req, res) => {
  const computer = Computer.findById(req.params.id);
  
  if (!computer) {
    if (req.headers['hx-request']) {
      return res.status(404).send('Computer not found');
    }
    return res.status(404).render('error.html', {
      message: 'Computer not found',
      status: 404
    });
  }
  
  const user = User.findByComputerAndUsername(computer.id, req.params.username);
  
  if (!user) {
    if (req.headers['hx-request']) {
      return res.status(404).send('User not found');
    }
    return res.status(404).render('error.html', {
      message: 'User not found',
      status: 404
    });
  }
  
  // Create enable command
  Command.create(computer.id, Command.TYPES.ENABLE_USER, user.username);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.ENABLE_USER,
    computer.hostname,
    user.username
  );
  
  // Return updated users list for HTMX
  if (req.headers['hx-request']) {
    const updatedComputer = Computer.getWithUsers(computer.id);
    const pendingCommands = Command.findPendingByComputerId(computer.id);
    return res.render('partials/users-list.html', { 
      computer: updatedComputer,
      pendingCommands
    });
  }
  
  res.redirect(`/computers/${computer.id}`);
});

// POST /computers/:id/user/:username/logout - Force logout a user
router.post('/:id/user/:username/logout', (req, res) => {
  const computer = Computer.findById(req.params.id);
  
  if (!computer) {
    if (req.headers['hx-request']) {
      return res.status(404).send('Computer not found');
    }
    return res.status(404).render('error.html', {
      message: 'Computer not found',
      status: 404
    });
  }
  
  const user = User.findByComputerAndUsername(computer.id, req.params.username);
  
  if (!user) {
    if (req.headers['hx-request']) {
      return res.status(404).send('User not found');
    }
    return res.status(404).render('error.html', {
      message: 'User not found',
      status: 404
    });
  }
  
  // Create logout command
  Command.create(computer.id, Command.TYPES.LOGOUT_USER, user.username);
  
  // Log the action
  AuditLog.log(
    req.session.user.id,
    req.session.user.username,
    AuditLog.ACTIONS.LOGOUT_USER,
    computer.hostname,
    user.username
  );
  
  // Return updated users list for HTMX
  if (req.headers['hx-request']) {
    const updatedComputer = Computer.getWithUsers(computer.id);
    const pendingCommands = Command.findPendingByComputerId(computer.id);
    return res.render('partials/users-list.html', { 
      computer: updatedComputer,
      pendingCommands
    });
  }
  
  res.redirect(`/computers/${computer.id}`);
});

module.exports = router;
