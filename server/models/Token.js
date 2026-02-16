const db = require('../config/database');
const { v4: uuidv4 } = require('uuid');

class Token {
  static findAll() {
    return db.all(`
      SELECT t.*, c.hostname as computer_hostname, c.last_seen_at as computer_last_seen_at
      FROM agent_tokens t
      LEFT JOIN computers c ON t.used_by_computer_id = c.id
      ORDER BY t.created_at DESC
    `);
  }

  static findById(id) {
    return db.get('SELECT * FROM agent_tokens WHERE id = ?', [id]);
  }

  static findByToken(token) {
    return db.get('SELECT * FROM agent_tokens WHERE token = ?', [token]);
  }

  static findAvailable() {
    return db.all(`
      SELECT * FROM agent_tokens 
      WHERE used_at IS NULL
      ORDER BY created_at DESC
    `);
  }

  static generate(name = null) {
    const token = uuidv4();
    const result = db.run(`
      INSERT INTO agent_tokens (token, name)
      VALUES (?, ?)
    `, [token, name]);
    
    return this.findById(result.lastInsertRowid);
  }

  static markUsed(token, computerId) {
    db.run(`
      UPDATE agent_tokens 
      SET used_at = datetime('now'),
          used_by_computer_id = ?
      WHERE token = ?
    `, [computerId, token]);
  }

  static isValid(token) {
    const tokenRecord = this.findByToken(token);
    return tokenRecord && !tokenRecord.used_at;
  }

  static delete(id) {
    const token = this.findById(id);
    if (token && token.used_by_computer_id) {
      // Don't delete tokens that are in use
      return false;
    }
    db.run('DELETE FROM agent_tokens WHERE id = ?', [id]);
    return true;
  }

  static revoke(id) {
    // Revoke a token (delete it even if used)
    db.run('DELETE FROM agent_tokens WHERE id = ?', [id]);
    return true;
  }
}

module.exports = Token;
