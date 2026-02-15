const express = require('express');
const router = express.Router();
const Token = require('../../models/Token');
const Computer = require('../../models/Computer');
const User = require('../../models/User');
const Command = require('../../models/Command');

// POST /api/agent/register - Register a new agent with a token
router.post('/register', (req, res) => {
  const { token, hostname, ip_address, agent_version } = req.body;
  
  if (!token || !hostname) {
    return res.status(400).json({ 
      error: 'Token and hostname are required' 
    });
  }
  
  // Validate token
  if (!Token.isValid(token)) {
    return res.status(401).json({ 
      error: 'Invalid or already used token' 
    });
  }
  
  // Check if computer with this hostname already exists
  let computer = Computer.findByHostname(hostname);
  
  if (computer) {
    // Update existing computer
    computer = Computer.updateHeartbeat(computer.id, {
      ip_address,
      agent_version,
      current_user: null
    });
  } else {
    // Create new computer
    computer = Computer.create({
      hostname,
      ip_address,
      agent_version
    });
  }
  
  // Mark token as used
  Token.markUsed(token, computer.id);
  
  res.json({
    status: 'ok',
    computer_id: computer.id,
    message: 'Agent registered successfully'
  });
});

// POST /api/agent/heartbeat - Agent heartbeat with status update
router.post('/heartbeat', (req, res) => {
  const { 
    computer_id, 
    hostname,
    ip_address, 
    agent_version,
    current_desktop_user,
    users 
  } = req.body;
  
  if (!computer_id) {
    return res.status(400).json({ 
      error: 'computer_id is required' 
    });
  }
  
  // Find computer
  const computer = Computer.findById(computer_id);
  
  if (!computer) {
    return res.status(404).json({ 
      error: 'Computer not found. Please re-register the agent.' 
    });
  }
  
  // Update computer heartbeat
  Computer.updateHeartbeat(computer_id, {
    ip_address: ip_address || computer.ip_address,
    agent_version: agent_version || computer.agent_version,
    current_user: current_desktop_user || null
  });
  
  // Sync users if provided
  if (users && Array.isArray(users)) {
    User.syncUsers(computer_id, users);
  }
  
  // Get pending commands for this computer
  const pendingCommands = Command.findPendingByComputerId(computer_id);
  
  // Mark commands as sent
  const commandsToSend = pendingCommands.map(cmd => {
    Command.markSent(cmd.id);
    return {
      id: cmd.id,
      type: cmd.command_type,
      target_user: cmd.target_user
    };
  });
  
  res.json({
    status: 'ok',
    commands: commandsToSend
  });
});

// POST /api/agent/commands/:id/result - Report command execution result
router.post('/commands/:id/result', (req, res) => {
  const commandId = req.params.id;
  const { success, error, message } = req.body;
  
  const command = Command.findById(commandId);
  
  if (!command) {
    return res.status(404).json({ 
      error: 'Command not found' 
    });
  }
  
  if (success) {
    Command.markCompleted(commandId, message || 'Success');
    
    // If it was a disable/enable command, update user status
    if (command.command_type === Command.TYPES.DISABLE_USER) {
      const user = User.findByComputerAndUsername(command.computer_id, command.target_user);
      if (user) {
        User.setLocked(user.id, true);
      }
    } else if (command.command_type === Command.TYPES.ENABLE_USER) {
      const user = User.findByComputerAndUsername(command.computer_id, command.target_user);
      if (user) {
        User.setLocked(user.id, false);
      }
    }
  } else {
    Command.markFailed(commandId, error || 'Unknown error');
  }
  
  res.json({
    status: 'ok'
  });
});

// GET /api/agent/commands - Get pending commands (alternative to heartbeat)
router.get('/commands', (req, res) => {
  const { computer_id } = req.query;
  
  if (!computer_id) {
    return res.status(400).json({ 
      error: 'computer_id is required' 
    });
  }
  
  const computer = Computer.findById(computer_id);
  
  if (!computer) {
    return res.status(404).json({ 
      error: 'Computer not found' 
    });
  }
  
  const pendingCommands = Command.findPendingByComputerId(computer_id);
  
  const commandsToSend = pendingCommands.map(cmd => {
    Command.markSent(cmd.id);
    return {
      id: cmd.id,
      type: cmd.command_type,
      target_user: cmd.target_user
    };
  });
  
  res.json({
    status: 'ok',
    commands: commandsToSend
  });
});

module.exports = router;
