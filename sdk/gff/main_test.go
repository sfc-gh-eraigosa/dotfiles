package main

import (
	"errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

func TestExitCodeNil(t *testing.T) {
	code, silent := exitCode(nil)
	if code != 0 {
		t.Errorf("exitCode(nil) code = %d, want 0", code)
	}
	if silent {
		t.Errorf("exitCode(nil) silent = %v, want false", silent)
	}
}

func TestExitCodeUnknownKey(t *testing.T) {
	err := resolve.ErrUnknownKey
	code, silent := exitCode(err)
	if code != 2 {
		t.Errorf("exitCode(ErrUnknownKey) code = %d, want 2", code)
	}
	if silent {
		t.Errorf("exitCode(ErrUnknownKey) silent = %v, want false", silent)
	}
}

func TestExitCodeUnknownOption(t *testing.T) {
	err := resolve.ErrUnknownOption
	code, silent := exitCode(err)
	if code != 2 {
		t.Errorf("exitCode(ErrUnknownOption) code = %d, want 2", code)
	}
	if silent {
		t.Errorf("exitCode(ErrUnknownOption) silent = %v, want false", silent)
	}
}

func TestExitCodeUnknownSource(t *testing.T) {
	err := resolve.ErrUnknownSource
	code, silent := exitCode(err)
	if code != 2 {
		t.Errorf("exitCode(ErrUnknownSource) code = %d, want 2", code)
	}
	if silent {
		t.Errorf("exitCode(ErrUnknownSource) silent = %v, want false", silent)
	}
}

func TestExitCodeWrongFlagType(t *testing.T) {
	err := resolve.ErrWrongFlagType
	code, silent := exitCode(err)
	if code != 2 {
		t.Errorf("exitCode(ErrWrongFlagType) code = %d, want 2", code)
	}
	if silent {
		t.Errorf("exitCode(ErrWrongFlagType) silent = %v, want false", silent)
	}
}

func TestExitCodeGenericError(t *testing.T) {
	err := errors.New("some random error")
	code, silent := exitCode(err)
	if code != 1 {
		t.Errorf("exitCode(generic) code = %d, want 1", code)
	}
	if silent {
		t.Errorf("exitCode(generic) silent = %v, want false", silent)
	}
}
