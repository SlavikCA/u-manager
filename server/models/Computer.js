const db = require('../config/database');

class Computer {
  static findAll() {
    return db.all(`
      SELECT * FROM computers 
      ORDER BY 
        CASE WHEN status = 'online' THEN 0 ELSE 1 END,
        hostname
    `);
  }

  static findById(id) {
    return db.get('SELECT * FROM computers WHERE id = ?', [id]);
  }

  static findByHostname(hostname) {
    return db.get('SELECT * FROM computers WHERE hostname = ?', [hostname]);
  }

  static create(data) {
    const result = db.run(`
      INSERT INTO computers (hostname, ip_address, agent_version, api_key_hash, status, last_seen_at)
      VALUES (?, ?, ?, ?, 'online', datetime('now'))
    `, [data.hostname, data.ip_address, data.agent_version, data.api_key_hash]);

    return this.findById(result.lastInsertRowid);
  }

  static setApiKeyHash(id, apiKeyHash) {
    db.run('UPDATE computers SET api_key_hash = ? WHERE id = ?', [apiKeyHash, id]);
  }

  static updateHeartbeat(id, data) {
    db.run(`
      UPDATE computers 
      SET ip_address = ?,
          agent_version = ?,
          current_user = ?,
          status = 'online',
          last_seen_at = datetime('now')
      WHERE id = ?
    `, [data.ip_address, data.agent_version, data.current_user, id]);
    
    return this.findById(id);
  }

  static markOffline(id) {
    db.run(`
      UPDATE computers 
      SET status = 'offline'
      WHERE id = ?
    `, [id]);
  }

  static markStaleComputersOffline(thresholdSeconds = 30) {
    // Mark computers as offline if they haven't been seen in the threshold time
    db.run(`
      UPDATE computers 
      SET status = 'offline'
      WHERE status = 'online' 
        AND datetime(last_seen_at, '+' || ? || ' seconds') < datetime('now')
    `, [thresholdSeconds]);
  }

  static getWithUsers(id) {
    const computer = this.findById(id);
    if (!computer) return null;

    const User = require('./User');
    computer.users = User.findByComputerId(id);
    
    return computer;
  }

  static delete(id) {
    db.run('DELETE FROM computers WHERE id = ?', [id]);
  }
}

module.exports = Computer;
