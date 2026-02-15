const Database = require('better-sqlite3');
const bcrypt = require('bcryptjs');
const fs = require('fs');
const path = require('path');

const dbPath = process.env.DATABASE_PATH || path.join(__dirname, '..', 'db', 'manager.db');
const dbDir = path.dirname(dbPath);

// Ensure db directory exists
if (!fs.existsSync(dbDir)) {
  fs.mkdirSync(dbDir, { recursive: true });
}

let db = null;

// Check if database needs initialization (new database)
function needsInitialization() {
  try {
    const result = db.prepare("SELECT name FROM sqlite_master WHERE type='table' AND name='admins'").get();
    return !result;
  } catch (e) {
    return true;
  }
}

// Initialize schema from schema.sql
function initializeSchema() {
  const schemaPath = path.join(__dirname, '..', 'scripts', 'schema.sql');
  if (fs.existsSync(schemaPath)) {
    const schema = fs.readFileSync(schemaPath, 'utf8');
    db.exec(schema);
    console.log('Database schema initialized');
  } else {
    console.error('Warning: schema.sql not found at', schemaPath);
  }
}

// Seed default admin user
function seedAdminUser() {
  const adminCount = db.prepare('SELECT COUNT(*) as count FROM admins').get();
  
  if (!adminCount || adminCount.count === 0) {
    const defaultUsername = process.env.ADMIN_USERNAME || 'admin';
    const defaultPassword = process.env.ADMIN_PASSWORD || 'admin123';
    
    const passwordHash = bcrypt.hashSync(defaultPassword, 10);
    
    db.prepare('INSERT INTO admins (username, password_hash) VALUES (?, ?)').run(defaultUsername, passwordHash);
    
    console.log('');
    console.log('='.repeat(50));
    console.log('Default admin user created:');
    console.log(`  Username: ${defaultUsername}`);
    console.log(`  Password: ${defaultPassword}`);
    console.log('');
    console.log('⚠️  IMPORTANT: Change this password immediately!');
    console.log('='.repeat(50));
    console.log('');
  }
}

// Initialize the database
function initDatabase() {
  if (db) return db;
  
  const isNewDatabase = !fs.existsSync(dbPath);
  
  db = new Database(dbPath);
  
  // Enable foreign keys and WAL mode for better performance
  db.pragma('foreign_keys = ON');
  db.pragma('journal_mode = WAL');
  
  // Auto-initialize if database is new or missing tables
  if (isNewDatabase || needsInitialization()) {
    console.log('New database detected, initializing...');
    initializeSchema();
    seedAdminUser();
  }

  // Run migrations for existing databases
  runMigrations();

  return db;
}

// Run migrations for existing databases
function runMigrations() {
  try {
    db.prepare("SELECT api_key_hash FROM computers LIMIT 1").get();
  } catch (e) {
    db.exec("ALTER TABLE computers ADD COLUMN api_key_hash TEXT");
    console.log('Migration: added api_key_hash column to computers');
  }
}

// Get the database instance
function getDatabase() {
  if (!db) {
    initDatabase();
  }
  return db;
}

// Helper function to run a query and return all results
function all(sql, params = []) {
  const stmt = db.prepare(sql);
  return stmt.all(...params);
}

// Helper function to run a query and return first result
function get(sql, params = []) {
  const stmt = db.prepare(sql);
  return stmt.get(...params);
}

// Helper function to run a query (INSERT, UPDATE, DELETE)
function run(sql, params = []) {
  const stmt = db.prepare(sql);
  const result = stmt.run(...params);
  return { lastInsertRowid: result.lastInsertRowid, changes: result.changes };
}

// Helper function to execute multiple statements
function exec(sql) {
  db.exec(sql);
}

// Close database on process exit
process.on('exit', () => {
  if (db) {
    db.close();
  }
});

process.on('SIGINT', () => {
  if (db) {
    db.close();
  }
  process.exit();
});

process.on('SIGTERM', () => {
  if (db) {
    db.close();
  }
  process.exit();
});

module.exports = {
  initDatabase,
  getDatabase,
  all,
  get,
  run,
  exec,
  getDb: () => db
};
