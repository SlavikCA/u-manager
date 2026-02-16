# Linux-users-management

SERVER offers web UI to manage (enable, disable) Linux users on workstations.
AGENT is binary program which runs as SystemD on workstation and comminicate with SERVER to report the status of users and receive commands to change user's settings.

# Installation

1. Deploy server. See [server/README.md](server/README.md)

2. Install agent on every workstation you want to manage:

```
curl -fsSL https://raw.githubusercontent.com/SlavikCA/u-manager/master/install-agent.sh | sudo bash
```

You can pin a version with 
```
LUM_VERSION=agent-v1.2.0 curl -fsSL https://raw.githubusercontent.com/SlavikCA/u-manager/master/install-agent.sh | sudo bash
```

The curl command can be used for upgrade.
