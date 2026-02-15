1. Deploy server. See [server/README.md](server/README.md)

2. Install agent on every workstation you want to manage:

```
curl -fsSL https://raw.githubusercontent.com/SlavikCA/u-manager/master/install-agent.sh | sudo bash
```

You can pin a version with 
```
LUM_VERSION=agent-v1.0.0 curl -fsSL https://raw.githubusercontent.com/SlavikCA/u-manager/master/install-agent.sh | sudo bash
```
