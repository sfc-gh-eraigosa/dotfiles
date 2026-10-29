package gh_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
)

func TestParsePR_HeadSHA(t *testing.T) {
	pr, err := gh.ParsePR([]byte(`{"number":7,"state":"MERGED","headRefName":"b","headRefOid":"abc123","url":"u"}`))
	if err != nil {
		t.Fatal(err)
	}
	if pr.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q", pr.HeadSHA)
	}
}
