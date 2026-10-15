package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newSSSDFixture points the deny-list code at a temporary sssd config so the
// tests never touch /etc.
func newSSSDFixture(t *testing.T, conf string, snippets map[string]string) {
	t.Helper()

	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Fatal(err)
	}

	confPath := filepath.Join(dir, "sssd.conf")
	if err := os.WriteFile(confPath, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	for name, content := range snippets {
		if err := os.WriteFile(filepath.Join(confDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	sssdConfPath, sssdConfDir = confPath, confDir
	t.Cleanup(func() {
		sssdConfPath = "/etc/sssd/sssd.conf"
		sssdConfDir = "/etc/sssd/conf.d"
		runCommand = defaultRunCommand
		lookPath = exec.LookPath
	})
}

func TestDenyDomains(t *testing.T) {
	tests := []struct {
		name     string
		conf     string
		snippets map[string]string
		want     []string
		wantErr  string
	}{
		{
			name: "simple access provider",
			conf: "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = simple\nsimple_allow_groups = linux-users, linux-admins\n",
			want: []string{"authentik"},
		},
		{
			name: "snippet overrides access provider",
			conf: "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = ldap\n",
			snippets: map[string]string{
				"01-override.conf": "[domain/authentik]\naccess_provider = simple\n",
			},
			want: []string{"authentik"},
		},
		{
			name:    "other access provider",
			conf:    "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = ldap\n",
			wantErr: `access_provider is "ldap"`,
		},
		{
			name:    "no access provider",
			conf:    "[sssd]\ndomains = authentik\n\n[domain/authentik]\nldap_uri = ldap://host\n",
			wantErr: `access_provider is "unset"`,
		},
		{
			name:    "allow users conflicts with deny users",
			conf:    "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = simple\nsimple_allow_users = alice\n",
			wantErr: "simple_allow_users is set",
		},
		{
			name: "deny list configured outside the generated snippet",
			conf: "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = simple\n",
			snippets: map[string]string{
				"10-admin.conf": "[domain/authentik]\nsimple_deny_users = bob\n",
			},
			wantErr: "10-admin.conf",
		},
		{
			name:    "no domains",
			conf:    "[sssd]\nservices = nss, pam\n",
			wantErr: "no domains listed",
		},
		{
			name: "mixed providers keeps the usable domain",
			conf: "[sssd]\ndomains = ad, authentik\n\n[domain/ad]\naccess_provider = ad\n\n[domain/authentik]\naccess_provider = simple\n",
			want: []string{"authentik"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newSSSDFixture(t, tc.conf, tc.snippets)

			domains, err := denyDomains(sssdConfPath, sssdConfDir, denySnippetName)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(domains, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("want domains %v, got %v", tc.want, domains)
			}
		})
	}
}

func TestWriteDenySnippet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.d", denySnippetName)

	denied := map[string]bool{"carol": true, "alice": true}
	if err := writeDenySnippet(path, []string{"authentik", "other"}, denied); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[domain/authentik]\nsimple_deny_users = alice, carol",
		"[domain/other]\nsimple_deny_users = alice, carol",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("snippet is missing %q:\n%s", want, content)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("want mode 0600, got %o", mode)
	}

	if err := writeDenySnippet(path, nil, map[string]bool{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("empty deny list should remove the snippet, stat returned %v", err)
	}
}

func TestApplyDenyList(t *testing.T) {
	conf := "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = simple\n"

	t.Run("writes list and restarts sssd", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)

		var calls []string
		runCommand = func(name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		}

		if err := denyDirectoryUser("alice"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertSnippetDenies(t, "alice")

		// A second disable keeps the first one: the snippet is read back as the
		// deny list before it is rewritten.
		if err := denyDirectoryUser("bob"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertSnippetDenies(t, "alice", "bob")

		restarted := false
		for _, call := range calls {
			if strings.Contains(call, "systemctl restart sssd") {
				restarted = true
			}
		}
		if !restarted {
			t.Fatalf("sssd was not restarted, calls: %v", calls)
		}

		// Enabling one account leaves the other denied.
		if err := allowDirectoryUser("alice"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertSnippetDenies(t, "bob")

		// Enabling the last account removes the snippet.
		if err := allowDirectoryUser("bob"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(denySnippetPath()); !os.IsNotExist(err) {
			t.Fatalf("snippet should be removed once nobody is denied, stat returned %v", err)
		}
	})

	t.Run("honours a hand-edited snippet", func(t *testing.T) {
		newSSSDFixture(t, conf, map[string]string{
			denySnippetName: "[domain/authentik]\nsimple_deny_users = carol\n",
		})

		if got := deniedUsernames(); !got["carol"] {
			t.Fatalf("hand-edited deny list is not honoured: %v", got)
		}

		runCommand = func(name string, args ...string) ([]byte, error) { return nil, nil }
		if err := denyDirectoryUser("alice"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertSnippetDenies(t, "alice", "carol")
	})

	t.Run("rolls back a rejected configuration", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)

		runCommand = func(name string, args ...string) ([]byte, error) {
			if strings.Contains(name, "sssctl") {
				return []byte("Invalid configuration"), errors.New("exit status 1")
			}
			return nil, nil
		}

		err := denyDirectoryUser("alice")
		if err == nil || !strings.Contains(err.Error(), "config-check failed") {
			t.Fatalf("want a config-check error, got %v", err)
		}
		if _, statErr := os.Stat(denySnippetPath()); !os.IsNotExist(statErr) {
			t.Fatalf("snippet should have been rolled back, stat returned %v", statErr)
		}
	})

	t.Run("rolls back to the previous list", func(t *testing.T) {
		newSSSDFixture(t, conf, map[string]string{
			denySnippetName: "[domain/authentik]\nsimple_deny_users = carol\n",
		})

		runCommand = func(name string, args ...string) ([]byte, error) {
			if strings.Contains(name, "sssctl") {
				return nil, errors.New("exit status 1")
			}
			return nil, nil
		}

		if err := denyDirectoryUser("alice"); err == nil {
			t.Fatal("want a config-check error")
		}
		assertSnippetDenies(t, "carol")
	})

	t.Run("rolls back when sssd cannot be restarted", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)

		runCommand = func(name string, args ...string) ([]byte, error) {
			if name == "systemctl" {
				return []byte("Job for sssd.service failed"), errors.New("exit status 1")
			}
			return nil, nil
		}

		err := denyDirectoryUser("alice")
		if err == nil || !strings.Contains(err.Error(), "failed to restart sssd") {
			t.Fatalf("want a restart error, got %v", err)
		}
		if _, statErr := os.Stat(denySnippetPath()); !os.IsNotExist(statErr) {
			t.Fatalf("snippet should have been rolled back, stat returned %v", statErr)
		}
	})
}

func assertSnippetDenies(t *testing.T, names ...string) {
	t.Helper()

	denied := deniedUsernames()
	for _, name := range names {
		if !denied[name] {
			t.Fatalf("%s should be denied, deny list is %v", name, denied)
		}
	}
	if len(denied) != len(names) {
		t.Fatalf("want exactly %v denied, got %v", names, denied)
	}
}

func TestDeniedUsernames(t *testing.T) {
	conf := "[sssd]\ndomains = authentik\n\n[domain/authentik]\naccess_provider = simple\n"

	t.Run("no snippet", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)
		if got := deniedUsernames(); len(got) != 0 {
			t.Fatalf("want an empty deny list, got %v", got)
		}
	})

	t.Run("all domains are merged", func(t *testing.T) {
		newSSSDFixture(t, conf, map[string]string{
			denySnippetName: "[domain/authentik]\nsimple_deny_users = alice, bob\n\n[domain/other]\nsimple_deny_users = carol\n",
		})

		got := deniedUsernames()
		for _, name := range []string{"alice", "bob", "carol"} {
			if !got[name] {
				t.Fatalf("%s is missing from %v", name, got)
			}
		}
		if len(got) != 3 {
			t.Fatalf("want 3 denied accounts, got %v", got)
		}
	})

	t.Run("separation by comma or space", func(t *testing.T) {
		newSSSDFixture(t, conf, map[string]string{
			denySnippetName: "[domain/authentik]\nsimple_deny_users = alice,bob carol\n",
		})

		got := deniedUsernames()
		for _, name := range []string{"alice", "bob", "carol"} {
			if !got[name] {
				t.Fatalf("%s is missing from %v", name, got)
			}
		}
	})
}

func TestLoginsDenied(t *testing.T) {
	tests := []struct {
		name         string
		sssctl       bool
		output       string
		wantBlocked  bool
		wantVerified bool
	}{
		{
			name:         "denied",
			sssctl:       true,
			output:       "user: alice\n\nSSSD nss user lookup result:\n\ntesting pam_acct_mgmt\n\npam_acct_mgmt: Permission denied\n",
			wantBlocked:  true,
			wantVerified: true,
		},
		{
			name:         "still accepted",
			sssctl:       true,
			output:       "testing pam_acct_mgmt\n\npam_acct_mgmt: Success\n",
			wantBlocked:  false,
			wantVerified: true,
		},
		{
			name:   "unexpected output",
			sssctl: true,
			output: "pam_start failed: Module is unknown\n",
		},
		{
			name:   "sssctl missing",
			sssctl: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookPath = func(name string) (string, error) {
				if tc.sssctl && name == "sssctl" {
					return "/usr/sbin/sssctl", nil
				}
				return "", errors.New("not found")
			}
			runCommand = func(name string, args ...string) ([]byte, error) {
				return []byte(tc.output), nil
			}
			t.Cleanup(func() {
				lookPath = exec.LookPath
				runCommand = defaultRunCommand
			})

			blocked, verified, output := loginsDenied("alice")
			if blocked != tc.wantBlocked || verified != tc.wantVerified {
				t.Fatalf("want blocked=%v verified=%v, got blocked=%v verified=%v (%s)",
					tc.wantBlocked, tc.wantVerified, blocked, verified, output)
			}
		})
	}
}
