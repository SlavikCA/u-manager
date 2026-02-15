const db = require('../config/database');
const bcrypt = require('bcryptjs');

class Admin {
  static findById(id) {
    return db.get('SELECT id, username, created_at FROM admins WHERE id = ?', [id]);
  }

  static findByUsername(username) {
    return db.get('SELECT * FROM admins WHERE username = ?', [username]);
  }

  static async authenticate(username, password) {
    const admin = this.findByUsername(username);
    if (!admin) {
      return null;
    }

    const isValid = await bcrypt.compare(password, admin.password_hash);
    if (!isValid) {
      return null;
    }

    // Return admin without password hash
    return {
      id: admin.id,
      username: admin.username,
      created_at: admin.created_at
    };
  }

  static async create(username, password) {
    const passwordHash = await bcrypt.hash(password, 10);
    const result = db.run(
      'INSERT INTO admins (username, password_hash) VALUES (?, ?)',
      [username, passwordHash]
    );
    return this.findById(result.lastInsertRowid);
  }

  static async updatePassword(id, newPassword) {
    const passwordHash = await bcrypt.hash(newPassword, 10);
    db.run('UPDATE admins SET password_hash = ? WHERE id = ?', [passwordHash, id]);
  }
}

module.exports = Admin;
