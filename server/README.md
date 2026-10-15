# Linux User Manager

A web application for managing Linux users across multiple computers. Built with Node.js, Express, SQLite, and HTMX.

## Features

- **Dashboard**: View all registered computers with online/offline status
- **User Management**: Disable, enable, and force logout users remotely
- **Agent Tokens**: Generate one-time tokens for registering new agents
- **Audit Log**: Track all administrative actions
- **Real-time Updates**: HTMX-powered auto-refresh every 10 seconds

## Quick Start

### Prerequisites

- Node.js 22+ 
- npm

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd manager
```

2. Install dependencies:
```bash
npm install
```

3. Start the server:
```bash
npm start
```

The database will be automatically initialized on first run with a default admin user:
- **Username**: `admin`
- **Password**: `admin123`

⚠️ **Change this password immediately in production!**

Or for development with auto-reload:
```bash
npm run dev
```

4. Open http://localhost:3000 in your browser

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 3000 | Server port |
| `HOST` | 0.0.0.0 | Server bind address |
| `SERVER_URL` | http://localhost:3000 | Public URL of the server (for agents) |
| `SESSION_SECRET` | (random) | Session encryption secret |
| `DATABASE_PATH` | `./db/manager.db` | SQLite database path |
| `NODE_ENV` | development | Environment (production/development) |
| `ADMIN_USERNAME` | admin | Default admin username (only used on first run) |
| `ADMIN_PASSWORD` | admin123 | Default admin password (only used on first run) |

### Example Production Configuration

```bash
export PORT=8080
export HOST=0.0.0.0
export SERVER_URL=https://manager.example.com
export SESSION_SECRET=your-secure-random-secret
export NODE_ENV=production
npm start
```

## Project Structure

```
manager/
├── server.js              # Main entry point
├── config/
│   └── database.js        # SQLite connection
├── middleware/
│   └── auth.js            # Authentication middleware
├── models/
│   ├── Admin.js           # Admin user model
│   ├── Computer.js        # Computer model
│   ├── User.js            # Linux user model
│   ├── Token.js           # Agent token model
│   ├── Command.js         # Pending command model
│   └── AuditLog.js        # Audit log model
├── routes/
│   ├── auth.js            # Login/logout routes
│   ├── index.js           # Dashboard routes
│   ├── computers.js       # Computer management
│   ├── tokens.js          # Token management
│   ├── audit.js           # Audit log routes
│   └── api/
│       └── agent.js       # Agent API endpoints
├── views/
│   ├── layouts/
│   │   └── base.html      # Base template
│   ├── partials/          # HTMX partials
│   ├── index.html         # Dashboard
│   ├── computer.html      # Computer management
│   ├── tokens.html        # Token management
│   ├── audit.html         # Audit log
│   └── login.html         # Login page
├── public/
│   ├── css/
│   │   └── style.css      # Styles
│   └── js/
│       └── htmx.min.js    # HTMX library
├── db/
│   └── schema.sql         # Database schema
├── scripts/
│   └── init-db.js         # Database initialization
├── docs/
│   └── agent-api.md       # Agent API documentation
└── plans/
    └── architecture.md    # Architecture documentation
```

## Usage

### 1. Generate Agent Token

1. Log in to the web UI
2. Go to **Tokens** page
3. Click **Generate Token**
4. Copy the token for use during agent installation

### 2. Install Agent on Linux Machine

See [Agent API Documentation](docs/agent-api.md) for details on implementing an agent.

The agent should:
1. Register with the server using the token
2. Send heartbeats every 10 seconds with:
   - Current desktop user
   - List of users (UID >= 1000) from `getent passwd`, so directory accounts
     (SSSD/LDAP, e.g. Authentik) are included
   - User lock status (`passwd -S` for local accounts, the sssd deny list for
     directory accounts)
3. Execute commands received from the server

### 3. Manage Users

1. From the Dashboard, click **Manage** on an online computer
2. View all users on that computer
3. Use the action buttons:
   - **Disable**: Prevent the user from logging in
   - **Enable**: Allow the user to log in again
   - **Logout**: End that user's sessions (console, RDP, ssh); other users keep working

#### Local and directory accounts

The agent lists every account NSS knows about (`getent passwd`), so accounts from
a remote directory (SSSD/LDAP, e.g. Authentik) appear next to local ones. Accounts
that are not in `/etc/passwd` are marked **Directory** in the user list.

Disabling means something different for the two kinds:

| Account | Disable | Enable |
|---------|---------|--------|
| Local (`/etc/passwd`) | `usermod -L`, this machine only | `usermod -U` |
| Directory (SSSD/LDAP) | Added to `simple_deny_users` in the machine's sssd configuration | Removed from that list |

A directory account cannot be locked with `usermod`: that tool only edits
`/etc/passwd`, and the account lives in the directory. The agent therefore
maintains `/etc/sssd/conf.d/99-linux-user-manager.conf` on every affected machine
and restarts sssd. This requires `access_provider = simple` for the domain; the
command fails with an explanation when the machine's sssd configuration does not
support it.

Note that a deny list is per machine. To block an account everywhere, deactivate
it in the directory itself (Authentik: *Users* → user → *Active* off). Doing that
is safe for this app: the LDAP outpost still returns deactivated accounts
(attribute `ak-active`), so they keep appearing in the user list and can be
re-enabled.

Failed commands are listed as **Recent Failures** in the users panel for 15
minutes, with the reason reported by the agent.

## API Endpoints

### Web UI Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Dashboard |
| GET | `/computers/:id` | Manage computer |
| POST | `/computers/:id/user/:username/disable` | Disable user |
| POST | `/computers/:id/user/:username/enable` | Enable user |
| POST | `/computers/:id/user/:username/logout` | Logout user |
| GET | `/tokens` | Token management |
| POST | `/tokens/generate` | Generate new token |
| GET | `/audit` | Audit log |

### Agent API Routes

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/agent/register` | Register new agent |
| POST | `/api/agent/heartbeat` | Agent heartbeat |
| POST | `/api/agent/commands/:id/result` | Report command result |

See [Agent API Documentation](docs/agent-api.md) for full details.

## Security Considerations

1. **HTTPS**: Use HTTPS in production (configure reverse proxy)
2. **Session Secret**: Set a strong `SESSION_SECRET` environment variable
3. **Change Default Password**: Change the default admin password immediately
4. **Firewall**: Restrict access to the management interface
5. **Agent Tokens**: Tokens are single-use; revoke unused tokens

## Development

### Running in Development Mode

```bash
npm run dev
```

This enables:
- Auto-reload on file changes
- Template watching

### Database Reset

To reset the database:
```bash
rm db/manager.db db/manager.db-wal db/manager.db-shm
npm start  # Database will be auto-initialized
```

Or use the init script:
```bash
rm db/manager.db db/manager.db-wal db/manager.db-shm
npm run init-db
```

## License

MIT
