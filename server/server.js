const express = require('express');
const session = require('express-session');
const SqliteStore = require('better-sqlite3-session-store')(session);
const nunjucks = require('nunjucks');
const path = require('path');

const db = require('./config/database');
const authMiddleware = require('./middleware/auth');

// Import routes
const authRoutes = require('./routes/auth');
const indexRoutes = require('./routes/index');
const computersRoutes = require('./routes/computers');
const tokensRoutes = require('./routes/tokens');
const auditRoutes = require('./routes/audit');
const agentRoutes = require('./routes/api/agent');

// Initialize database first (synchronous with better-sqlite3)
db.initDatabase();
console.log('Database initialized');

const app = express();

// Environment configuration
const PORT = process.env.PORT || 3000;
const HOST = process.env.HOST || '0.0.0.0';
const SERVER_URL = process.env.SERVER_URL || `http://localhost:${PORT}`;

// Configure Nunjucks templating
nunjucks.configure('views', {
  autoescape: true,
  express: app,
  watch: false // Disable watch to avoid chokidar dependency
});
app.set('view engine', 'html');

// Middleware
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
app.use(express.static(path.join(__dirname, 'public')));

// Session configuration with SQLite store for persistence across restarts
app.use(session({
  store: new SqliteStore({
    client: db.getDatabase(),
    expired: {
      clear: true,
      intervalMs: 900000 // Clear expired sessions every 15 minutes
    }
  }),
  secret: process.env.SESSION_SECRET || 'change-this-secret-in-production',
  resave: false,
  saveUninitialized: false,
  rolling: true,
  cookie: {
    secure: process.env.NODE_ENV === 'production',
    httpOnly: true,
    maxAge: 7 * 24 * 60 * 60 * 1000 // 7 days
  }
}));

// Make session user and server URL available to all templates
app.use((req, res, next) => {
  res.locals.user = req.session.user || null;
  res.locals.serverUrl = SERVER_URL;
  next();
});

// Routes
app.use('/auth', authRoutes);
app.use('/api/agent', agentRoutes);

// Protected routes (require authentication)
app.use('/', authMiddleware, indexRoutes);
app.use('/computers', authMiddleware, computersRoutes);
app.use('/tokens', authMiddleware, tokensRoutes);
app.use('/audit', authMiddleware, auditRoutes);

// Error handling middleware
app.use((err, req, res, next) => {
  console.error('Error:', err);
  
  if (req.path.startsWith('/api/')) {
    return res.status(err.status || 500).json({
      error: err.message || 'Internal server error'
    });
  }
  
  res.status(err.status || 500).render('error.html', {
    message: err.message || 'Something went wrong',
    status: err.status || 500
  });
});

// 404 handler
app.use((req, res) => {
  if (req.path.startsWith('/api/')) {
    return res.status(404).json({ error: 'Not found' });
  }
  res.status(404).render('error.html', {
    message: 'Page not found',
    status: 404
  });
});

// Start server
app.listen(PORT, HOST, () => {
  console.log(`Linux User Manager running on ${SERVER_URL}`);
  console.log(`Listening on ${HOST}:${PORT}`);
});

module.exports = app;
