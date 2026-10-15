const db = require('../config/database');

class User {
  static findById(id) {
    return db.get('SELECT * FROM users WHERE id = ?', [id]);
  }

  static findByComputerId(computerId) {
    return db.all(`
      SELECT * FROM users
      WHERE computer_id = ?
      ORDER BY
        CASE
          WHEN is_locked = 1 THEN 2
          WHEN is_logged_in = 1 THEN 0
          ELSE 1
        END,
        username ASC
    `, [computerId]);
  }

  static findByComputerAndUsername(computerId, username) {
    return db.get(`
      SELECT * FROM users 
      WHERE computer_id = ? AND username = ?
    `, [computerId, username]);
  }

  static upsert(computerId, userData) {
    const existing = this.findByComputerAndUsername(computerId, userData.username);
    const isLocal = userData.is_local === undefined ? true : userData.is_local;
    const loginAt = this.loginTimestamp(userData);
    
    if (existing) {
      db.run(`
        UPDATE users 
        SET uid = ?,
            home_dir = ?,
            shell = ?,
            is_local = ?,
            is_locked = ?,
            is_logged_in = ?,
            last_login_at = COALESCE(?, last_login_at),
            updated_at = datetime('now')
        WHERE id = ?
      `, [
        userData.uid,
        userData.home_dir,
        userData.shell,
        isLocal ? 1 : 0,
        userData.is_locked ? 1 : 0,
        userData.is_logged_in ? 1 : 0,
        loginAt,
        existing.id
      ]);
      return this.findById(existing.id);
    } else {
      const result = db.run(`
        INSERT INTO users (computer_id, username, uid, home_dir, shell, is_local, is_locked, is_logged_in, last_login_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
      `, [
        computerId,
        userData.username,
        userData.uid,
        userData.home_dir,
        userData.shell,
        isLocal ? 1 : 0,
        userData.is_locked ? 1 : 0,
        userData.is_logged_in ? 1 : 0,
        loginAt
      ]);
      return this.findById(result.lastInsertRowid);
    }
  }

  // Convert the agent-reported login epoch (seconds) to the UTC
  // "YYYY-MM-DD HH:MM:SS" format used by last_login_at; null when the
  // agent did not report a login time (older agents or not logged in).
  static loginTimestamp(userData) {
    if (!userData.logged_in_at) return null;
    const d = new Date(userData.logged_in_at * 1000);
    if (Number.isNaN(d.getTime())) return null;
    return d.toISOString().slice(0, 19).replace('T', ' ');
  }

  static syncUsers(computerId, users) {
    // Get current usernames from the incoming data
    const incomingUsernames = users.map(u => u.username);
    
    // Remove users that no longer exist on the computer
    if (incomingUsernames.length > 0) {
      const placeholders = incomingUsernames.map(() => '?').join(',');
      db.run(`
        DELETE FROM users 
        WHERE computer_id = ? AND username NOT IN (${placeholders})
      `, [computerId, ...incomingUsernames]);
    } else {
      // If no users provided, remove all users for this computer
      db.run('DELETE FROM users WHERE computer_id = ?', [computerId]);
    }
    
    // Upsert each user
    for (const userData of users) {
      this.upsert(computerId, userData);
    }
    
    return this.findByComputerId(computerId);
  }

  static setLocked(id, isLocked) {
    db.run(`
      UPDATE users 
      SET is_locked = ?,
          updated_at = datetime('now')
      WHERE id = ?
    `, [isLocked ? 1 : 0, id]);
    return this.findById(id);
  }

  static updateLoginStatus(computerId, username, isLoggedIn) {
    db.run(`
      UPDATE users 
      SET is_logged_in = ?,
          last_login_at = CASE WHEN ? = 1 THEN datetime('now') ELSE last_login_at END,
          updated_at = datetime('now')
      WHERE computer_id = ? AND username = ?
    `, [isLoggedIn ? 1 : 0, isLoggedIn ? 1 : 0, computerId, username]);
  }
}

module.exports = User;
