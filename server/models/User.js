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
    
    if (existing) {
      db.run(`
        UPDATE users 
        SET uid = ?,
            home_dir = ?,
            shell = ?,
            is_locked = ?,
            is_logged_in = ?,
            updated_at = datetime('now')
        WHERE id = ?
      `, [
        userData.uid,
        userData.home_dir,
        userData.shell,
        userData.is_locked ? 1 : 0,
        userData.is_logged_in ? 1 : 0,
        existing.id
      ]);
      return this.findById(existing.id);
    } else {
      const result = db.run(`
        INSERT INTO users (computer_id, username, uid, home_dir, shell, is_locked, is_logged_in)
        VALUES (?, ?, ?, ?, ?, ?, ?)
      `, [
        computerId,
        userData.username,
        userData.uid,
        userData.home_dir,
        userData.shell,
        userData.is_locked ? 1 : 0,
        userData.is_logged_in ? 1 : 0
      ]);
      return this.findById(result.lastInsertRowid);
    }
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
