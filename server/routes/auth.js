const express = require('express');
const router = express.Router();
const Admin = require('../models/Admin');
const AuditLog = require('../models/AuditLog');

// GET /auth/login - Show login page
router.get('/login', (req, res) => {
  if (req.session.user) {
    return res.redirect('/');
  }
  res.render('login.html', { error: null });
});

// POST /auth/login - Process login
router.post('/login', async (req, res) => {
  const { username, password } = req.body;
  
  if (!username || !password) {
    return res.render('login.html', { 
      error: 'Username and password are required' 
    });
  }
  
  try {
    const admin = await Admin.authenticate(username, password);
    
    if (!admin) {
      return res.render('login.html', { 
        error: 'Invalid username or password' 
      });
    }
    
    // Set session
    req.session.user = admin;
    
    // Log the login
    AuditLog.log(admin.id, admin.username, AuditLog.ACTIONS.LOGIN);
    
    // Redirect to original URL or dashboard
    const returnTo = req.session.returnTo || '/';
    delete req.session.returnTo;
    
    res.redirect(returnTo);
  } catch (error) {
    console.error('Login error:', error);
    res.render('login.html', { 
      error: 'An error occurred during login' 
    });
  }
});

// POST /auth/logout - Process logout
router.post('/logout', (req, res) => {
  if (req.session.user) {
    AuditLog.log(
      req.session.user.id, 
      req.session.user.username, 
      AuditLog.ACTIONS.LOGOUT
    );
  }
  
  req.session.destroy((err) => {
    if (err) {
      console.error('Logout error:', err);
    }
    res.redirect('/auth/login');
  });
});

// GET /auth/logout - Also support GET for convenience
router.get('/logout', (req, res) => {
  if (req.session.user) {
    AuditLog.log(
      req.session.user.id, 
      req.session.user.username, 
      AuditLog.ACTIONS.LOGOUT
    );
  }
  
  req.session.destroy((err) => {
    if (err) {
      console.error('Logout error:', err);
    }
    res.redirect('/auth/login');
  });
});

module.exports = router;
