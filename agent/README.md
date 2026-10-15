# Linux User Manager Agent

## User management

The agent reports every account NSS knows about (UID >= 1000, via
`getent passwd`), so accounts from a remote directory such as Authentik over
SSSD show up in the web UI next to local ones.

Disabling and enabling an account depends on where it lives:

- **Local accounts** (`/etc/passwd`): `usermod -L` / `usermod -U`.
- **Directory accounts** (SSSD/LDAP, reported as `is_local: false`): `usermod`
  cannot touch them, because it only edits `/etc/passwd`. The agent instead
  writes `simple_deny_users` into
  `/etc/sssd/conf.d/99-linux-user-manager.conf` for every sssd domain with
  `access_provider = simple`, validates the file with `sssctl config-check`,
  restarts sssd (config changes are only picked up on start) and then checks
  through PAM (`sssctl user-checks -a acct -s <service> <user>`) that the account
  is really refused. A configuration sssd rejects is rolled back.

`sssctl config-check` validates the whole configuration, so it also reports what
`sssd.conf` itself got wrong, and the option lists the validator ships with are
older than the daemon's (on Ubuntu `ldap_auth_disable_tls_never_use_in_production`
and `config_file_version` are flagged although the daemon accepts them). Only
problems the generated snippet *adds* block the operation; problems that were
already there are logged and ignored, since they would otherwise make disabling
directory accounts impossible on a machine whose SSSD works.

The snippet is the only state the agent keeps: `simple_deny_users` in it is the
deny list, so a hand edit is honoured on the next heartbeat. A denied account
that NSS stops enumerating is looked up individually (and reported as locked) so
it keeps its row on the server; an account the directory no longer returns at all
is dropped from the report. The deny is per machine: the account itself stays
active in the directory.

The checks use `sssctl` from the `sssd-tools` package. Without it the agent still
writes the snippet, but cannot validate it, and reports the disable as unverified;
install `sssd-tools` on the managed machines.

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
git tag agent-v2.0.2
git push --tags
```

Then check it in Github:
https://github.com/SlavikCA/u-manager/releases
