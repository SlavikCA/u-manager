const db = require('../config/database');

class Command {
  static TYPES = {
    DISABLE_USER: 'disable_user',
    ENABLE_USER: 'enable_user',
    LOGOUT_USER: 'logout_user'
  };

  static STATUS = {
    PENDING: 'pending',
    SENT: 'sent',
    COMPLETED: 'completed',
    FAILED: 'failed'
  };

  static findById(id) {
    return db.get('SELECT * FROM pending_commands WHERE id = ?', [id]);
  }

  static findPendingByComputerId(computerId) {
    return db.all(`
      SELECT * FROM pending_commands
      WHERE computer_id = ? AND status = 'pending'
      ORDER BY created_at ASC
    `, [computerId]);
  }

  static findByComputerId(computerId, limit = 50) {
    return db.all(`
      SELECT * FROM pending_commands 
      WHERE computer_id = ?
      ORDER BY created_at DESC
      LIMIT ?
    `, [computerId, limit]);
  }

  static create(computerId, commandType, targetUser) {
    const result = db.run(`
      INSERT INTO pending_commands (computer_id, command_type, target_user)
      VALUES (?, ?, ?)
    `, [computerId, commandType, targetUser]);
    
    return this.findById(result.lastInsertRowid);
  }

  static markSent(id) {
    db.run(`
      UPDATE pending_commands 
      SET status = 'sent'
      WHERE id = ?
    `, [id]);
    return this.findById(id);
  }

  static markCompleted(id, result = null) {
    db.run(`
      UPDATE pending_commands 
      SET status = 'completed',
          executed_at = datetime('now'),
          result = ?
      WHERE id = ?
    `, [result, id]);
    return this.findById(id);
  }

  static markFailed(id, error) {
    db.run(`
      UPDATE pending_commands 
      SET status = 'failed',
          executed_at = datetime('now'),
          result = ?
      WHERE id = ?
    `, [error, id]);
    return this.findById(id);
  }

  static getPendingCount(computerId) {
    const result = db.get(`
      SELECT COUNT(*) as count FROM pending_commands
      WHERE computer_id = ? AND status = 'pending'
    `, [computerId]);
    return result ? result.count : 0;
  }

  static cancelPending(computerId, targetUser = null) {
    if (targetUser) {
      db.run(`
        DELETE FROM pending_commands 
        WHERE computer_id = ? AND target_user = ? AND status = 'pending'
      `, [computerId, targetUser]);
    } else {
      db.run(`
        DELETE FROM pending_commands 
        WHERE computer_id = ? AND status = 'pending'
      `, [computerId]);
    }
  }
}

module.exports = Command;
