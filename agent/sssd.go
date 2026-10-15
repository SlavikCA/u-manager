package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Directory accounts (SSSD/LDAP, e.g. Authentik) have no entry in /etc/passwd,
// so usermod cannot lock them. Disabling one therefore means denying logins in
// the machine's own SSSD configuration: a generated snippet in
// /etc/sssd/conf.d with simple_deny_users, which works with
// "access_provider = simple" and overrides any allow list (see sssd-simple(5):
// deny rules supersede matched allow rules).
//
// The snippet is the only state: SSSD reads it, the agent reads and rewrites it,
// and an administrator editing it by hand is honoured on the next heartbeat.
//
// SSSD re-reads its configuration only on start (SIGHUP merely rotates logs),
// so every change is followed by a restart of sssd.
var (
	sssdConfPath    = "/etc/sssd/sssd.conf"
	sssdConfDir     = "/etc/sssd/conf.d"
	denySnippetName = "99-linux-user-manager.conf"
)

// runCommand is indirected so tests can stub the externals the deny list
// depends on. LC_ALL is forced to C so parsed sssctl output is not localized.
var runCommand = defaultRunCommand

// lookPath is indirected for the same reason.
var lookPath = exec.LookPath

func defaultRunCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd.CombinedOutput()
}

// deniedUsernames reads the accounts this machine denies logins for, from the
// generated snippet: its simple_deny_users entries are the deny list.
func deniedUsernames() map[string]bool {
	names := map[string]bool{}

	sections := map[string]map[string]iniOption{}
	if err := mergeIniFile(sections, denySnippetPath()); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to read %s: %v", denySnippetPath(), err)
		}
		return names
	}
	for _, options := range sections {
		for _, name := range splitSSSDList(options["simple_deny_users"].value) {
			names[name] = true
		}
	}
	return names
}

// denyDirectoryUser adds a directory account to this machine's deny list.
func denyDirectoryUser(username string) error {
	denied := deniedUsernames()
	if denied[username] {
		return nil
	}
	denied[username] = true
	return applyDenyList(denied)
}

// allowDirectoryUser removes a directory account from this machine's deny list.
func allowDirectoryUser(username string) error {
	denied := deniedUsernames()
	if !denied[username] {
		return nil
	}
	delete(denied, username)
	return applyDenyList(denied)
}

// applyDenyList writes the wanted deny list, validates it and restarts SSSD. The
// snippet is rolled back if any step fails, so a rejected configuration never
// survives on disk.
func applyDenyList(denied map[string]bool) error {
	var domains []string
	if len(denied) > 0 {
		var err error
		if domains, err = denyDomains(sssdConfPath, sssdConfDir, denySnippetName); err != nil {
			return err
		}
	}

	previous := snapshotFile(denySnippetPath())
	rollback := func() {
		if err := previous.restore(); err != nil {
			log.Printf("Failed to restore %s: %v", previous.path, err)
		}
	}

	// sssctl config-check validates the whole configuration, so it also reports
	// what the administrator's own sssd.conf got wrong, and on Ubuntu the
	// validator's option lists are older than the daemon's (an option the
	// daemon accepts, ldap_auth_disable_tls_never_use_in_production for one, is
	// not listed there). Such problems never go away on their own, so only the
	// problems the deny list adds may block the operation.
	existing, checked := sssdConfigProblems()
	if !checked {
		log.Printf("sssctl not found, skipping configuration check")
	}
	if len(existing) > 0 {
		log.Printf("sssctl config-check already reports %d problem(s) without the deny list, ignoring: %s",
			len(existing), strings.Join(existing, " "))
	}

	if err := writeDenySnippet(denySnippetPath(), domains, denied); err != nil {
		return err
	}
	if checked {
		now, _ := sssdConfigProblems()
		if added := addedProblems(existing, now); len(added) > 0 {
			rollback()
			return fmt.Errorf("sssctl config-check rejected the deny list: %s", strings.Join(added, " "))
		}
	}
	if err := restartSSSD(); err != nil {
		rollback()
		if restartErr := restartSSSD(); restartErr != nil {
			return fmt.Errorf("%v (restoring the previous configuration also failed: %v)", err, restartErr)
		}
		return err
	}

	if len(denied) > 0 {
		log.Printf("Denied logins for %d user(s) in sssd domain(s) %s", len(denied), strings.Join(domains, ", "))
	} else {
		log.Printf("Deny list is empty, removed %s", denySnippetPath())
	}
	return nil
}

func denySnippetPath() string {
	return filepath.Join(sssdConfDir, denySnippetName)
}

// writeDenySnippet renders the generated sssd snippet. With an empty deny list
// the snippet is removed entirely: simpler than keeping domains around for a
// list nobody is on.
func writeDenySnippet(path string, domains []string, denied map[string]bool) error {
	if len(denied) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	names := make([]string, 0, len(denied))
	for name := range denied {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Generated by lum-agent (Linux User Manager). Do not edit:\n")
	b.WriteString("# the agent rewrites this file whenever a directory user is enabled or disabled.\n")
	for _, domain := range domains {
		fmt.Fprintf(&b, "\n[domain/%s]\nsimple_deny_users = %s\n", domain, strings.Join(names, ", "))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return err
	}

	// SSSD only reads snippets with the same owner and permissions as
	// sssd.conf (root:root, 0600 by default).
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 0, 0); err != nil {
			return err
		}
	}
	return nil
}

// iniOption is a configuration option value plus the file it came from, so
// errors can name the file the administrator has to edit.
type iniOption struct {
	value string
	file  string
}

// denyDomains returns the SSSD domains of this machine that accept a deny list.
func denyDomains(confPath, confDir, snippetName string) ([]string, error) {
	sections, err := readSSSDConfig(confPath, confDir, snippetName)
	if err != nil {
		return nil, err
	}

	domains := splitSSSDList(sections["sssd"]["domains"].value)
	if len(domains) == 0 {
		return nil, fmt.Errorf("no domains listed in [sssd] domains of %s", confPath)
	}

	var usable, blocked []string
	for _, domain := range domains {
		options := sections["domain/"+domain]
		provider := strings.ToLower(options["access_provider"].value)
		switch {
		case provider != "simple":
			if provider == "" {
				provider = "unset"
			}
			blocked = append(blocked, fmt.Sprintf("domain %s: access_provider is %q, only \"simple\" honors a deny list", domain, provider))
		case strings.TrimSpace(options["simple_allow_users"].value) != "":
			blocked = append(blocked, fmt.Sprintf("domain %s: simple_allow_users is set, and sssd rejects simple_deny_users together with it", domain))
		default:
			usable = append(usable, domain)
		}
	}
	if len(usable) == 0 {
		return nil, fmt.Errorf("cannot deny logins through sssd: %s", strings.Join(blocked, "; "))
	}

	// The generated snippet sorts last, so it would silently override a deny
	// list set elsewhere. Refuse instead of dropping the administrator's entries.
	for _, domain := range usable {
		if option, ok := sections["domain/"+domain]["simple_deny_users"]; ok && strings.TrimSpace(option.value) != "" {
			return nil, fmt.Errorf("simple_deny_users is already set in %s; remove it there before disabling directory users with this app", option.file)
		}
	}

	return usable, nil
}

// readSSSDConfig merges sssd.conf and the conf.d snippets, later files winning
// as documented in sssd.conf(5). The agent's own snippet is skipped: it is the
// output of this package, not an administrator's input.
func readSSSDConfig(confPath, confDir, snippetName string) (map[string]map[string]iniOption, error) {
	if _, err := os.Stat(confPath); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", confPath, err)
	}

	files := []string{confPath}
	snippets, err := filepath.Glob(filepath.Join(confDir, "*.conf"))
	if err != nil {
		return nil, err
	}
	sort.Strings(snippets)
	for _, snippet := range snippets {
		if filepath.Base(snippet) == snippetName {
			continue
		}
		files = append(files, snippet)
	}

	sections := map[string]map[string]iniOption{}
	for _, file := range files {
		if err := mergeIniFile(sections, file); err != nil {
			return nil, err
		}
	}
	return sections, nil
}

func mergeIniFile(sections map[string]map[string]iniOption, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[] "))
			if sections[section] == nil {
				sections[section] = map[string]iniOption{}
			}
			continue
		}
		if section == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		sections[section][strings.ToLower(strings.TrimSpace(key))] = iniOption{
			value: strings.TrimSpace(value),
			file:  path,
		}
	}
	return nil
}

func splitSSSDList(value string) []string {
	var values []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

// sssdConfigProblems returns the problems sssctl config-check reports for the
// configuration currently on disk, with the volatile parts of the report (the
// issue count, the list of snippet files) removed so that two runs are
// comparable. checked is false when sssctl is not installed.
func sssdConfigProblems() (problems []string, checked bool) {
	path, err := lookPath("sssctl")
	if err != nil {
		return nil, false
	}

	out, err := runCommand(path, "config-check")
	if err == nil {
		return nil, true
	}

	inSnippetList := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case inSnippetList && strings.HasPrefix(line, "/"):
			// File names of the snippets that were merged, one per line.
			continue
		case strings.HasPrefix(line, "Issues identified by validators:"),
			strings.HasPrefix(line, "Messages generated during configuration merging:"),
			strings.HasPrefix(line, "Used configuration snippet files:"):
			inSnippetList = strings.HasPrefix(line, "Used configuration snippet files:")
			continue
		default:
			inSnippetList = false
			problems = append(problems, line)
		}
	}
	return problems, true
}

// addedProblems returns the problems in after that were not in before, so a
// report that only repeats what the configuration already got wrong is not
// mistaken for a problem introduced by a change.
func addedProblems(before, after []string) []string {
	remaining := make(map[string]int, len(before))
	for _, problem := range before {
		remaining[problem]++
	}

	var added []string
	for _, problem := range after {
		if remaining[problem] > 0 {
			remaining[problem]--
			continue
		}
		added = append(added, problem)
	}
	return added
}

// restartSSSD is the only way to apply configuration changes: SIGHUP just
// reopens the log files.
func restartSSSD() error {
	out, err := runCommand("systemctl", "restart", "sssd")
	if err != nil {
		return fmt.Errorf("failed to restart sssd: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// loginsDenied asks sssd through PAM whether the account may still log in.
// verified is false when the check could not run at all (no sssctl, no usable
// PAM service, unexpected output); output explains either case.
func loginsDenied(username string) (blocked bool, verified bool, output string) {
	path, err := lookPath("sssctl")
	if err != nil {
		return false, false, "sssctl not found"
	}
	service := pamService()
	if service == "" {
		return false, false, "no usable PAM service in /etc/pam.d"
	}

	out, _ := runCommand(path, "user-checks", "-a", "acct", "-s", service, username)
	text := string(out)
	for _, line := range strings.Split(text, "\n") {
		if result, ok := strings.CutPrefix(strings.TrimSpace(line), "pam_acct_mgmt:"); ok {
			return !strings.EqualFold(strings.TrimSpace(result), "Success"), true, strings.TrimSpace(line)
		}
	}

	log.Printf("sssctl user-checks for %s reported no pam_acct_mgmt result:\n%s", username, text)
	return false, false, "sssctl user-checks reported no pam_acct_mgmt result (see agent log)"
}

// pamService picks a PAM service that exists on this machine, so user-checks
// can run an account phase. The services installed by sshd/login include the
// pam_sss module that consults the access provider.
func pamService() string {
	for _, service := range []string{"sshd", "login", "common-account", "system-auth"} {
		if _, err := os.Stat(filepath.Join("/etc/pam.d", service)); err == nil {
			return service
		}
	}
	return ""
}

// fileSnapshot allows a failed operation to be undone.
type fileSnapshot struct {
	path    string
	data    []byte
	existed bool
}

func snapshotFile(path string) fileSnapshot {
	data, err := os.ReadFile(path)
	return fileSnapshot{path: path, data: data, existed: err == nil}
}

func (s fileSnapshot) restore() error {
	if !s.existed {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(s.path, s.data, 0600)
}
