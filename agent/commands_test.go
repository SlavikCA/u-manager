package main

import (
	"errors"
	"strings"
	"testing"
)

// stubRunCommand replaces the command runner the logout uses and records every
// call, so the tests assert the command line without running it.
func stubRunCommand(t *testing.T, out string, err error) *[]string {
	t.Helper()

	var calls []string
	previous := runCommand
	runCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte(out), err
	}
	t.Cleanup(func() { runCommand = previous })

	return &calls
}

func TestRunLogoutUserTerminatesSessions(t *testing.T) {
	calls := stubRunCommand(t, "", nil)

	result := runLogoutUser("alice")

	if len(*calls) != 1 || (*calls)[0] != "loginctl terminate-user alice" {
		t.Fatalf("commands = %v, want [loginctl terminate-user alice]", *calls)
	}
	if !result.Success {
		t.Fatalf("want success, got %+v", result)
	}
	if !strings.Contains(result.Message, "alice") {
		t.Errorf("message %q should name the user", result.Message)
	}
}

// A user who is not logged in has no session to terminate. logind reports that
// as "is not logged in or lingering"; the request itself did not fail.
func TestRunLogoutUserWithoutSessions(t *testing.T) {
	stubRunCommand(t,
		"Could not terminate user: User ID 1001 is not logged in or lingering\n",
		errors.New("exit status 1"))

	result := runLogoutUser("bob")

	if !result.Success {
		t.Fatalf("want success for a user without sessions, got %+v", result)
	}
	if !strings.Contains(result.Message, "no active sessions") {
		t.Errorf("message = %q, want it to say there were no sessions", result.Message)
	}
	if result.Error != "" {
		t.Errorf("error = %q, want none", result.Error)
	}
}

// An account the machine does not know cannot be logged out; the reason logind
// gives is what the UI shows as the failure.
func TestRunLogoutUserUnknownUser(t *testing.T) {
	stubRunCommand(t,
		"Failed to look up user ghost: No such process\n",
		errors.New("exit status 1"))

	result := runLogoutUser("ghost")

	if result.Success {
		t.Fatalf("want failure for an unknown user, got %+v", result)
	}
	if !strings.Contains(result.Error, "ghost") || !strings.Contains(result.Error, "No such process") {
		t.Errorf("error = %q, want it to name the user and logind's reason", result.Error)
	}
}

// Commands aimed at a service account are refused instead of run, and nothing
// on the machine is touched.
func TestExecuteRefusesServiceAccounts(t *testing.T) {
	calls := stubRunCommand(t, "", nil)

	for _, cmd := range []Command{
		{ID: 1, Type: "disable_user", TargetUser: "service"},
		{ID: 2, Type: "enable_user", TargetUser: "service-backup"},
		{ID: 3, Type: "logout_user", TargetUser: "Service"},
	} {
		result := execute(cmd)

		if result.Success {
			t.Errorf("%s for %s should be refused, got %+v", cmd.Type, cmd.TargetUser, result)
		}
		if !strings.Contains(result.Error, cmd.TargetUser) {
			t.Errorf("error = %q, want it to name %s", result.Error, cmd.TargetUser)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("no command should have run, got %v", *calls)
	}
}

func TestExecuteRunsCommandsForManagedUsers(t *testing.T) {
	calls := stubRunCommand(t, "", nil)

	result := execute(Command{ID: 4, Type: "logout_user", TargetUser: "alice"})

	if !result.Success {
		t.Fatalf("want success, got %+v", result)
	}
	if len(*calls) != 1 || (*calls)[0] != "loginctl terminate-user alice" {
		t.Fatalf("commands = %v, want [loginctl terminate-user alice]", *calls)
	}
}
