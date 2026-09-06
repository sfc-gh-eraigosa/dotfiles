package updexec

import (
	"reflect"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// One quoting implementation for every provider and every update script:
// ShQuote is runner.Quote, not a second copy that could drift on an edge case.
func TestShQuoteIsRunnerQuote(t *testing.T) {
	if reflect.ValueOf(ShQuote).Pointer() != reflect.ValueOf(runner.Quote).Pointer() {
		t.Fatal("updexec.ShQuote must be the same function as runner.Quote")
	}
}
