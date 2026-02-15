-- Linux User Manager Database Schema

-- Admin users for web UI authentication
CREATE TABLE IF NOT EXISTS admins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now'))
);

-- Agent registration tokens
CREATE TABLE IF NOT EXISTS agent_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT UNIQUE NOT NULL,
    name TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    used_at TEXT,
    used_by_computer_id INTEGER,
    FOREIGN KEY (used_by_computer_id) REFERENCES computers(id)
);

-- Registered computers/machines
CREATE TABLE IF NOT EXISTS computers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname TEXT NOT NULL,
    ip_address TEXT,
    agent_version TEXT,
    last_seen_at TEXT,
    current_user TEXT,
    status TEXT DEFAULT 'offline',
    created_at TEXT DEFAULT (datetime('now'))
);

-- Users on each computer (UID >= 1000)
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    computer_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    uid INTEGER NOT NULL,
    home_dir TEXT,
    shell TEXT,
    is_locked INTEGER DEFAULT 0,
    is_logged_in INTEGER DEFAULT 0,
    last_login_at TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (computer_id) REFERENCES computers(id) ON DELETE CASCADE,
    UNIQUE(computer_id, username)
);

-- Pending commands for agents to execute
CREATE TABLE IF NOT EXISTS pending_commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    computer_id INTEGER NOT NULL,
    command_type TEXT NOT NULL,
    target_user TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    created_at TEXT DEFAULT (datetime('now')),
    executed_at TEXT,
    result TEXT,
    FOREIGN KEY (computer_id) REFERENCES computers(id) ON DELETE CASCADE
);

-- Audit log for tracking admin actions
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id INTEGER,
    admin_username TEXT,
    action TEXT NOT NULL,
    target_computer TEXT,
    target_user TEXT,
    details TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (admin_id) REFERENCES admins(id)
);

-- Indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_computers_status ON computers(status);
CREATE INDEX IF NOT EXISTS idx_computers_last_seen ON computers(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_users_computer_id ON users(computer_id);
CREATE INDEX IF NOT EXISTS idx_pending_commands_computer_id ON pending_commands(computer_id);
CREATE INDEX IF NOT EXISTS idx_pending_commands_status ON pending_commands(status);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
