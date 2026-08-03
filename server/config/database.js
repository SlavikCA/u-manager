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
  
  // Enable foreign keys and DELETE journal mode
  // DELETE mode is correct for single-process better-sqlite3:
  // WAL mode is unnecessary here (no concurrent readers/writers)
  // and can cause corruption when checkpoints silently fail.
  db.pragma('foreign_keys = ON');
  db.pragma('journal_mode = DELETE');
  
  // Auto-initialize if database is new or missing tables
  if (isNewDatabase || needsInitialization()) {
    console.log('New database detected, initializing...');
    initializeSchema();
    seedAdminUser();
    console.log('Database initialized');
  } else {
    console.log('Existing database loaded');
  }

  // Run migrations for existing databases
  runMigrations();

  return db;
}

// Run migrations for existing databases
function runMigrations() {
  // Migration: pending_commands.target_user must allow NULL.
  // The "Restart DM" command has no target user, but the original schema declared
  // target_user TEXT NOT NULL, so Command.create(computer.id, RESTART_DM, null)
  // threw a SqliteError and the command was never queued.
  // SQLite cannot ALTER a column, so rebuild the table (the standard 12-step procedure).
  const pc = db.prepare("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'pending_commands'").get();
  const needsTargetUserMigration = pc && pc.sql && /target_user\s+TEXT\s+NOT\s+NULL/i.test(pc.sql);

  if (needsTargetUserMigration) {
    console.log('Migration: making pending_commands.target_user nullable...');

    const fkWasOn = db.pragma('foreign_keys', { simple: true }) === 1;
    if (fkWasOn) db.pragma('foreign_keys = OFF');

    db.transaction(() => {
      db.exec(`
        CREATE TABLE pending_commands_new (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            computer_id INTEGER NOT NULL,
            command_type TEXT NOT NULL,
            target_user TEXT,
            status TEXT DEFAULT 'pending',
            created_at TEXT DEFAULT (datetime('now')),
            executed_at TEXT,
            result TEXT,
            FOREIGN KEY (computer_id) REFERENCES computers(id) ON DELETE CASCADE
        );

        INSERT INTO pending_commands_new
          (id, computer_id, command_type, target_user, status, created_at, executed_at, result)
          SELECT id, computer_id, command_type, target_user, status, created_at, executed_at, result
          FROM pending_commands;

        DROP TABLE pending_commands;
        ALTER TABLE pending_commands_new RENAME TO pending_commands;

        CREATE INDEX idx_pending_commands_computer_id ON pending_commands(computer_id);
        CREATE INDEX idx_pending_commands_status ON pending_commands(status);
      `);
    })();

    if (fkWasOn) db.pragma('foreign_keys = ON');

    console.log('Migration complete: pending_commands.target_user is now nullable');
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
