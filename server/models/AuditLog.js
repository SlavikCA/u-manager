const db = require('../config/database');

class AuditLog {
  static ACTIONS = {
    LOGIN: 'login',
    LOGOUT: 'logout',
    DISABLE_USER: 'disable_user',
    ENABLE_USER: 'enable_user',
    LOGOUT_USER: 'logout_user',
    GENERATE_TOKEN: 'generate_token',
    REVOKE_TOKEN: 'revoke_token',
    CHANGE_PASSWORD: 'change_password'
  };

  static findAll(limit = 100, offset = 0) {
    return db.all(`
      SELECT * FROM audit_log 
      ORDER BY created_at DESC
      LIMIT ? OFFSET ?
    `, [limit, offset]);
  }

  static findByAdmin(adminId, limit = 50) {
    return db.all(`
      SELECT * FROM audit_log 
      WHERE admin_id = ?
      ORDER BY created_at DESC
      LIMIT ?
    `, [adminId, limit]);
  }

  static findByComputer(computerHostname, limit = 50) {
    return db.all(`
      SELECT * FROM audit_log 
      WHERE target_computer = ?
      ORDER BY created_at DESC
      LIMIT ?
    `, [computerHostname, limit]);
  }

  static log(adminId, adminUsername, action, targetComputer = null, targetUser = null, details = null) {
    const parts = [`[AUDIT] action=${action} admin=${adminUsername}`];
    if (targetComputer) parts.push(`computer=${targetComputer}`);
    if (targetUser) parts.push(`user=${targetUser}`);
    if (details) parts.push(`details=${details}`);
    console.log(parts.join(' '));

    const result = db.run(`
      INSERT INTO audit_log (admin_id, admin_username, action, target_computer, target_user, details)
      VALUES (?, ?, ?, ?, ?, ?)
    `, [adminId, adminUsername, action, targetComputer, targetUser, details]);

    return result.lastInsertRowid;
  }

  static getCount() {
    const result = db.get('SELECT COUNT(*) as count FROM audit_log');
    return result ? result.count : 0;
  }

  static getRecentActivity(hours = 24) {
    return db.all(`
      SELECT * FROM audit_log 
      WHERE datetime(created_at) > datetime('now', '-' || ? || ' hours')
      ORDER BY created_at DESC
    `, [hours]);
  }

  static cleanup(daysToKeep = 90) {
    db.run(`
      DELETE FROM audit_log 
      WHERE datetime(created_at) < datetime('now', '-' || ? || ' days')
    `, [daysToKeep]);
  }
}

module.exports = AuditLog;
