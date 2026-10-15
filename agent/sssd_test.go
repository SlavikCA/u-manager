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

	t.Run("rolls back a configuration the snippet breaks", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)

		runCommand = func(name string, args ...string) ([]byte, error) {
			if !strings.Contains(name, "sssctl") {
				return nil, nil
			}
			if _, err := os.Stat(denySnippetPath()); err == nil {
				return []byte("Issues identified by validators: 1\n" +
					"[rule/allowed_domain_options]: Attribute 'simple_deny_users' is not allowed in section 'domain/authentik'. Check for typos.\n"), errors.New("exit status 1")
			}
			return nil, nil
		}

		err := denyDirectoryUser("alice")
		if err == nil || !strings.Contains(err.Error(), "rejected the deny list") {
			t.Fatalf("want a config-check error, got %v", err)
		}
		if _, statErr := os.Stat(denySnippetPath()); !os.IsNotExist(statErr) {
			t.Fatalf("snippet should have been rolled back, stat returned %v", statErr)
		}
	})

	t.Run("ignores problems that predate the snippet", func(t *testing.T) {
		newSSSDFixture(t, conf, nil)

		// What sssctl reports on a working Ubuntu machine: the validator's
		// option lists are older than the daemon's, so it flags options the
		// daemon accepts. Those problems are here to stay and must not block
		// the deny list.
		runCommand = func(name string, args ...string) ([]byte, error) {
			if !strings.Contains(name, "sssctl") {
				return nil, nil
			}
			return []byte(preexistingReport), errors.New("exit status 1")
		}

		if err := denyDirectoryUser("stephen"); err != nil {
			t.Fatalf("a pre-existing config-check problem must not block the deny list: %v", err)
		}
		assertSnippetDenies(t, "stephen")
	})

	t.Run("rolls back to the previous list", func(t *testing.T) {
		newSSSDFixture(t, conf, map[string]string{
			denySnippetName: "[domain/authentik]\nsimple_deny_users = carol\n",
		})

		// The list on disk is accepted, adding alice makes the validator
		// complain, so the rewrite has to be undone.
		runCommand = func(name string, args ...string) ([]byte, error) {
			if !strings.Contains(name, "sssctl") {
				return nil, nil
			}
			data, _ := os.ReadFile(denySnippetPath())
			if strings.Contains(string(data), "alice") {
				return []byte("Issues identified by validators: 1\n" +
					"[rule/allowed_domain_options]: Attribute 'simple_deny_users' is not allowed in section 'domain/authentik'. Check for typos.\n"), errors.New("exit status 1")
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

// preexistingReport is real sssctl output from an Ubuntu machine whose SSSD
// works: the validator's option lists ship without options the daemon accepts
// (ldap_auth_disable_tls_never_use_in_production) and without
// config_file_version, so an otherwise healthy configuration never passes
// config-check.
const preexistingReport = "Issues identified by validators: 2\n" +
	"[rule/allowed_sssd_options]: Attribute 'config_file_version' is not allowed in section 'sssd'. Check for typos.\n" +
	"[rule/allowed_domain_options]: Attribute 'ldap_auth_disable_tls_never_use_in_production' is not allowed in section 'domain/authentik'. Check for typos.\n" +
	"\nMessages generated during configuration merging: 0\n" +
	"\nUsed configuration snippet files: 1\n" +
	"/etc/sssd/conf.d/99-linux-user-manager.conf\n"

func TestSSSDConfigProblems(t *testing.T) {
	t.Cleanup(func() {
		lookPath = exec.LookPath
		runCommand = defaultRunCommand
	})
	lookPath = func(name string) (string, error) { return "/usr/sbin/sssctl", nil }

	t.Run("keeps the issues and drops the counts and snippet list", func(t *testing.T) {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return []byte(preexistingReport), errors.New("exit status 1")
		}

		problems, checked := sssdConfigProblems()
		if !checked {
			t.Fatal("sssctl is installed, want checked")
		}
		want := []string{
			"[rule/allowed_sssd_options]: Attribute 'config_file_version' is not allowed in section 'sssd'. Check for typos.",
			"[rule/allowed_domain_options]: Attribute 'ldap_auth_disable_tls_never_use_in_production' is not allowed in section 'domain/authentik'. Check for typos.",
		}
		if strings.Join(problems, "\n") != strings.Join(want, "\n") {
			t.Fatalf("want %v, got %v", want, problems)
		}
	})

	t.Run("keeps configuration merge errors", func(t *testing.T) {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return []byte(preexistingReport +
				"Messages generated during configuration merging: 3\n" +
				"Errors detected while parsing: /etc/sssd/conf.d/99-linux-user-manager.conf.\n" +
				"Error (5) on line 1: Equal sign is missing.\n" +
				"Due to errors file /etc/sssd/conf.d/99-linux-user-manager.conf is not considered. Skipping.\n"), errors.New("exit status 1")
		}

		problems, checked := sssdConfigProblems()
		if !checked {
			t.Fatal("sssctl is installed, want checked")
		}
		if len(problems) != 5 || !strings.Contains(problems[2], "Errors detected while parsing") {
			t.Fatalf("merge errors were dropped: %v", problems)
		}
	})

	t.Run("no sssctl", func(t *testing.T) {
		lookPath = func(name string) (string, error) { return "", errors.New("not found") }
		t.Cleanup(func() { lookPath = func(name string) (string, error) { return "/usr/sbin/sssctl", nil } })

		problems, checked := sssdConfigProblems()
		if checked || len(problems) != 0 {
			t.Fatalf("want no problems and checked=false, got %v, %v", problems, checked)
		}
	})

	t.Run("clean configuration", func(t *testing.T) {
		runCommand = func(name string, args ...string) ([]byte, error) {
			return []byte("Issues identified by validators: 0\n"), nil
		}

		problems, checked := sssdConfigProblems()
		if !checked || len(problems) != 0 {
			t.Fatalf("want no problems and checked=true, got %v, %v", problems, checked)
		}
	})
}

func TestAddedProblems(t *testing.T) {
	tests := []struct {
		name          string
		before, after []string
		want          []string
	}{
		{"nothing new", []string{"a", "b"}, []string{"b", "a"}, nil},
		{"dropped problems are not additions", []string{"a", "b"}, []string{"a"}, nil},
		{"repeated problem is counted", []string{"a"}, []string{"a", "a"}, []string{"a"}},
		{"new problem", []string{"a"}, []string{"a", "b"}, []string{"b"}},
		{"everything is new", nil, []string{"b"}, []string{"b"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := addedProblems(tc.before, tc.after); strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
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
