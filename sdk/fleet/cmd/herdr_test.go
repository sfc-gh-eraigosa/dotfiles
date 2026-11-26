package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

type fakeHerdrList struct {
	out   string
	err   error
	calls int
}

func (f *fakeHerdrList) Output(context.Context, []string) ([]byte, error) {
	f.calls++
	return []byte(f.out), f.err
}

// syncFixture: "me" is this machine, "saved" is already a herdr machine,
// "fresh" is not, and "other" is in ssh config but not in the fleet.
func syncFixture(tty bool) (herdrSyncDeps, *fakeHerdrList, *[][]string) {
	list := &fakeHerdrList{out: `[{"id":"1","label":"renamed","target":"saved"}]`}
	var adds [][]string
	d := herdrSyncDeps{
		allHosts: func() ([]sshconf.Host, error) {
			return []sshconf.Host{
				{Alias: "me", HostName: "10.0.0.1", Fleet: true},
				{Alias: "saved", HostName: "10.0.0.2", Fleet: true},
				{Alias: "fresh", HostName: "10.0.0.3", Fleet: true},
				{Alias: "other", HostName: "10.0.0.4"},
			}, nil
		},
		local:    func() localHost { return localHost{Name: "me-box", IPs: []string{"10.0.0.1"}} },
		herdrBin: func() string { return "/opt/herdr" },
		list:     list,
		tty:      func() bool { return tty },
		add: func(argv []string) error {
			adds = append(adds, argv)
			return nil
		},
	}
	return d, list, &adds
}

// F5a: --dry-run prints the add command for the one unsaved, non-local host
// and runs nothing.
func TestSyncDryRun(t *testing.T) {
	d, _, adds := syncFixture(true)
	var out bytes.Buffer
	if err := runHerdrSync(context.Background(), d, nil, true, false, &out); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(*adds) != 0 {
		t.Fatalf("dry-run executed %q", *adds)
	}
	got := out.String()
	if strings.Count(got, "herdr machine add") != 1 || !strings.Contains(got, "herdr machine add fresh --label fresh") {
		t.Fatalf("want exactly one add line, for fresh:\n%s", got)
	}
	if strings.Contains(got, "me ") || strings.Contains(got, "other") {
		t.Fatalf("the local host and non-fleet hosts are not part of the default set:\n%s", got)
	}
	if !strings.Contains(got, "already") {
		t.Fatalf("the saved host should be reported as already:\n%s", got)
	}
}

// F5b: with no TTY, an unsaved host is not attempted; the output names the
// command to run and the exit is non-zero.
func TestSyncWithoutTTYReportsNeedsApproval(t *testing.T) {
	d, _, adds := syncFixture(false)
	var out bytes.Buffer
	err := runHerdrSync(context.Background(), d, nil, false, false, &out)
	var ee exitError
	if !errors.As(err, &ee) || ee.code == 0 {
		t.Fatalf("want a non-zero exitError, got %v", err)
	}
	if len(*adds) != 0 {
		t.Fatalf("ran machine add without a TTY: %q", *adds)
	}
	if got := out.String(); !strings.Contains(got, "needs-approval") || !strings.Contains(got, "herdr machine add fresh --label fresh") {
		t.Fatalf("output must say needs-approval and name the command:\n%s", got)
	}
}

// With a TTY, unsaved hosts are added through the resolved local binary;
// one failure does not stop the rest, and makes the exit non-zero.
func TestSyncAddsWithTTYAndKeepsGoingAfterAFailure(t *testing.T) {
	d, list, adds := syncFixture(true)
	d.allHosts = func() ([]sshconf.Host, error) {
		return []sshconf.Host{
			{Alias: "bad", Fleet: true}, {Alias: "fresh", Fleet: true}, {Alias: "saved", Fleet: true},
		}, nil
	}
	inner := d.add
	d.add = func(argv []string) error {
		_ = inner(argv)
		if argv[3] == "bad" {
			return errors.New("exit status 1")
		}
		return nil
	}
	var out bytes.Buffer
	err := runHerdrSync(context.Background(), d, nil, false, true, &out)
	var ee exitError
	if !errors.As(err, &ee) {
		t.Fatalf("one failed host must make the exit non-zero, got %v", err)
	}
	want := [][]string{
		{"/opt/herdr", "machine", "add", "bad", "--label", "bad"},
		{"/opt/herdr", "machine", "add", "fresh", "--label", "fresh"},
	}
	if !reflect.DeepEqual(*adds, want) {
		t.Fatalf("adds %q, want %q", *adds, want)
	}
	if list.calls != 1 {
		t.Fatalf("machine list ran %d times, want once", list.calls)
	}
	var rows []herdrSyncResult
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Host] = r.Result
	}
	if !reflect.DeepEqual(got, map[string]string{"bad": "failed", "fresh": "saved", "saved": "already"}) {
		t.Fatalf("results %v", got)
	}
}

// F5c: --json is an array of {host,result,detail}; explicit hosts limit the
// set; an unknown alias is failed with "not in ssh config"; naming this
// machine is failed too.
func TestSyncJSONShapeAndExplicitHosts(t *testing.T) {
	d, _, adds := syncFixture(true)
	var out bytes.Buffer
	err := runHerdrSync(context.Background(), d, []string{"saved", "nope", "me", "other"}, false, true, &out)
	if err == nil {
		t.Fatal("an unknown alias must make the exit non-zero")
	}
	var raw []map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(raw) != 4 {
		t.Fatalf("want 4 rows, got %d: %v", len(raw), raw)
	}
	for _, r := range raw {
		if len(r) != 3 || r["host"] == nil || r["result"] == nil || r["detail"] == nil {
			t.Fatalf("row shape %v, want exactly host/result/detail", r)
		}
	}
	byHost := map[string]map[string]any{}
	for _, r := range raw {
		byHost[r["host"].(string)] = r
	}
	if byHost["saved"]["result"] != "already" {
		t.Errorf("saved: %v", byHost["saved"])
	}
	if byHost["nope"]["result"] != "failed" || !strings.Contains(byHost["nope"]["detail"].(string), "not in ssh config") {
		t.Errorf("nope: %v", byHost["nope"])
	}
	if byHost["me"]["result"] != "failed" || !strings.Contains(byHost["me"]["detail"].(string), "this machine") {
		t.Errorf("me: %v", byHost["me"])
	}
	// A named host outside the fleet is still honoured: naming it is consent.
	if byHost["other"]["result"] != "saved" {
		t.Errorf("other: %v", byHost["other"])
	}
	if len(*adds) != 1 || (*adds)[0][3] != "other" {
		t.Errorf("adds %q, want only other", *adds)
	}
}

// No local herdr is an error before anything is listed or added.
func TestSyncWithoutLocalHerdrIsAnError(t *testing.T) {
	d, list, adds := syncFixture(true)
	d.herdrBin = func() string { return "" }
	err := runHerdrSync(context.Background(), d, nil, false, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "herdr is not installed here") {
		t.Fatalf("got %v", err)
	}
	if list.calls != 0 || len(*adds) != 0 {
		t.Fatal("touched herdr without a binary")
	}
}

// If the saved machines cannot be read, nothing is added: a duplicate add
// would prompt, and we cannot tell what is saved.
func TestSyncRefusesWhenMachineListFails(t *testing.T) {
	d, list, adds := syncFixture(true)
	list.err = errors.New("exit status 7")
	err := runHerdrSync(context.Background(), d, nil, false, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("got %v", err)
	}
	if len(*adds) != 0 {
		t.Fatalf("added without knowing what is saved: %q", *adds)
	}
}

// The command is wired under `fleet herdr sync` with --dry-run.
func TestHerdrSyncCommandIsRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"herdr", "sync"})
	if err != nil || c.Name() != "sync" || c.Parent().Name() != "herdr" {
		t.Fatalf("fleet herdr sync not registered: %v %v", c, err)
	}
	if c.Flags().Lookup("dry-run") == nil {
		t.Fatal("--dry-run missing")
	}
}
