package gh

// PR is the gss view of a GitHub pull request. It is populated from the
// `gh pr view`/`gh pr list --json …` output (see ParsePR / ParsePRs) and
// is the single shape every higher layer reads — no package above this one
// ever decodes gh JSON itself.
//
// Field mapping to gh's --json keys:
//
//	Number    <- number
//	Title     <- title
//	Body      <- body
//	State     <- state        (OPEN | CLOSED | MERGED)
//	IsDraft   <- isDraft
//	Mergeable <- mergeable     (MERGEABLE | CONFLICTING | UNKNOWN)
//	Base      <- baseRefName
//	Head      <- headRefName
//	URL       <- url
type PR struct {
	Number    int
	Title     string
	Body      string
	State     string
	IsDraft   bool
	Mergeable string
	Base      string
	Head      string
	URL       string
}

// PRCreateOpts is the input to Client.PRCreate. It maps onto
//
//	gh pr create --title <Title> --base <Base> --head <Head> [--draft] \
//	             (--body-file <BodyFile> | --body <Body>)
//
// Title, Base and Head are required; an empty value for any of them is a
// caller bug and PRCreate rejects it before shelling out.
//
// Body vs BodyFile: the design (design.md → "GitHub interaction") prefers
// --body-file so long, marker-bearing markdown bodies don't have to round-
// trip through argv. When BodyFile is set it wins; otherwise Body is passed
// inline as a convenience for short bodies and tests.
type PRCreateOpts struct {
	Title    string
	Body     string
	BodyFile string
	Base     string
	Head     string
	Draft    bool
}

// PREditOpts is the input to Client.PREdit. Empty fields are left
// unchanged, so callers can retarget a base without touching the body and
// vice-versa. As with PRCreate, BodyFile wins over Body. At least one field
// must be set; an all-empty edit is rejected.
type PREditOpts struct {
	Base     string
	Body     string
	BodyFile string
}

// PRFilter is the input to Client.PRList. The zero value lists OPEN PRs
// (matching gh's own default). State is one of open | closed | merged | all.
// Head/Base, when set, restrict to PRs with that head/base branch; Limit
// caps the result count (0 = gh's default).
type PRFilter struct {
	State string
	Head  string
	Base  string
	Limit int
}
