const fs = require('fs');
const path = require('path');
const bcrypt = require('bcryptjs');

function initDatabase() {
  // Ensure db directory exists
  const dbDir = path.join(__dirname, '..', 'db');
  if (!fs.existsSync(dbDir)) {
    fs.mkdirSync(dbDir, { recursive: true });
  }

  const db = require('../config/database');
  
  console.log('Initializing database...');
  
  // Initialize the database connection
  db.initDatabase();
  
  // Read and execute schema
  const schemaPath = path.join(__dirname, 'schema.sql');
  const schema = fs.readFileSync(schemaPath, 'utf8');
  
  db.exec(schema);
  console.log('Schema created successfully.');

  // Create default admin user if none exists
  const adminCount = db.get('SELECT COUNT(*) as count FROM admins');

  if (!adminCount || adminCount.count === 0) {
    const defaultUsername = 'admin';
    const defaultPassword = 'admin123'; // Change this in production!
    
    const passwordHash = bcrypt.hashSync(defaultPassword, 10);
    
    db.run(
      'INSERT INTO admins (username, password_hash) VALUES (?, ?)',
      [defaultUsername, passwordHash]
    );
    
    console.log('');
    console.log('='.repeat(50));
    console.log('Default admin user created:');
    console.log(`  Username: ${defaultUsername}`);
    console.log(`  Password: ${defaultPassword}`);
    console.log('');
    console.log('⚠️  IMPORTANT: Change this password immediately!');
    console.log('='.repeat(50));
  } else {
    console.log('Admin user already exists, skipping creation.');
  }
  
  console.log('');
  console.log('Database initialization complete!');
  
  process.exit(0);
}

try {
  initDatabase();
} catch (err) {
  console.error('Error initializing database:', err);
  process.exit(1);
}
