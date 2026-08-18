# Linux User Manager Agent

## Screenshot capture

Screenshot capture is opt-in at registration:

```
sudo lum-agent --server-url http://SERVER:3000 --token TOKEN --screenshots
```

The capture method is selected with `--screenshot-method` (stored as
`screenshot_method` in `/etc/linux-user-manager/config.json`, default `xlib`):

- `xlib` — grabs the X11 display directly from the Go binary (works on Debian).
- `console` — runs the bundled `console-shot.sh` (loginctl + runuser + scrot),
  which screenshots the active console session as that session's user
  (works on Ubuntu). Requires `scrot` to be installed on the target machine.

Example for Ubuntu:

```
sudo lum-agent --server-url http://SERVER:3000 --token TOKEN --screenshots --screenshot-method console
```

## Publishing a release

```
git tag agent-v1.12.0
git push --tags
```

Then check it in Github:
https://github.com/SlavikCA/u-manager/releases
