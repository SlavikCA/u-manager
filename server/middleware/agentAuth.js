const crypto = require('crypto');
const Computer = require('../models/Computer');

function agentAuth(req, res, next) {
  const authHeader = req.headers['authorization'];

  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Missing or invalid Authorization header' });
  }

  const apiKey = authHeader.slice(7);
  const apiKeyHash = crypto.createHash('sha256').update(apiKey).digest('hex');

  const computerId = req.body.computer_id || req.query.computer_id;

  if (!computerId) {
    return res.status(400).json({ error: 'computer_id is required' });
  }

  const computer = Computer.findById(computerId);

  if (!computer || computer.api_key_hash !== apiKeyHash) {
    return res.status(401).json({ error: 'Invalid API key' });
  }

  req.computer = computer;
  next();
}

module.exports = agentAuth;
