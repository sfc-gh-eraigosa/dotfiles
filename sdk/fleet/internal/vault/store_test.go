package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSecretNeverPrints(t *testing.T) {
	s := Secret("PRIVATE-MATERIAL-123")
	j, err := json.Marshal(struct{ S Secret }{s})
	if err != nil {
		t.Fatal(err)
	}
	outs := []string{
		fmt.Sprint(s),
		fmt.Sprintf("%v %+v %#v %s %q %x", s, s, s, s, s, s),
		string(j),
		fmt.Sprintf("%+v", Blob{Data: s}),
	}
	for i, o := range outs {
		if !strings.Contains(o, "[redacted]") {
			t.Errorf("case %d: %q lacks [redacted]", i, o)
		}
		if strings.Contains(o, "PRIVATE-MATERIAL") {
			t.Errorf("case %d: %q leaks the secret", i, o)
		}
	}
}

func TestFakeCompareAndSet(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	d := Secret("PRIVATE-MATERIAL-123")
	v, err := f.Put(ctx, "n", d, "")
	if err != nil || v != "1" {
		t.Fatalf("create: %q %v", v, err)
	}
	if _, err := f.Put(ctx, "n", d, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("recreate: %v", err)
	}
	v, err = f.Put(ctx, "n", d, "1")
	if err != nil || v != "2" {
		t.Fatalf("update: %q %v", v, err)
	}
	if _, err := f.Put(ctx, "n", d, "1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := f.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	b, err := f.Get(ctx, "n")
	if err != nil || b.Version != "2" || string(b.Data) != "PRIVATE-MATERIAL-123" {
		t.Fatalf("get: %+v %v", b, err)
	}
	names, _ := f.List(ctx)
	if len(names) != 1 || names[0] != "n" {
		t.Fatalf("list: %v", names)
	}
	for _, c := range f.Calls {
		if strings.Contains(c, "PRIVATE") {
			t.Fatalf("call log leaks data: %q", c)
		}
	}
	if f.Name() == "" || f.Ready(ctx) != nil || f.SignIn(ctx, nil, nil, nil) != nil {
		t.Fatal("fake name/ready/signin")
	}
	if _, err := f.Put(ctx, "absent", d, "7"); !errors.Is(err, ErrConflict) {
		t.Fatalf("update of absent item: %v", err)
	}
}
