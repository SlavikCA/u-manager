/**
 * Authentication middleware for admin web UI
 * Requires user to be logged in via session
 */
function authMiddleware(req, res, next) {
  if (!req.session.user) {
    // For HTMX requests, return 401 with redirect header
    if (req.headers['hx-request']) {
      res.set('HX-Redirect', '/auth/login');
      return res.status(401).send('');
    }
    
    // Store the original URL to redirect back after login
    req.session.returnTo = req.originalUrl;
    return res.redirect('/auth/login');
  }
  
  next();
}

module.exports = authMiddleware;
