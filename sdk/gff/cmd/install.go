package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register and snapshot the CWD repo's feature flags",
	Long: `Install registers the current git repository under its declared namespace
and writes a user-layer snapshot for cross-repo flag resolution.

The namespace is taken from the flag file's "namespace:" field. If a
remote.origin.url is configured, the origin-derived namespace is computed
and a warning is printed when it differs (forks keep upstream identity).`,
	Args: cobra.NoArgs,
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, _ []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}

	// Discover the repo root from the working directory.
	repoRoot, ok := gitx.RepoRoot(r.P.WorkDir)
	if !ok {
		return fmt.Errorf("install: not inside a git repository (WorkDir=%s)", r.P.WorkDir)
	}

	// Locate the feature file.
	featPath := gitx.SourcePath(r.R, repoRoot)

	// Load and lint the feature file.
	ff, err := schema.LoadFeatureFile(featPath)
	if err != nil {
		return fmt.Errorf("install: load feature file: %w", err)
	}
	if findings := schema.Lint(ff); len(findings) > 0 {
		for _, f := range findings {
			fmt.Fprintf(cmd.ErrOrStderr(), "lint: %s [%s] %s\n", f.Path, f.Rule, f.Msg)
		}
		return fmt.Errorf("install: feature file has %d lint finding(s)", len(findings))
	}

	namespace := ff.GetNamespace()

	// Get the remote URL (tolerate absence).
	url, _ := r.R.Output(repoRoot, "config", "--get", "remote.origin.url")
	url = strings.TrimSpace(url)

	// Derive and compare origin namespace when a URL is available.
	if url != "" {
		derived := deriveNamespace(url)
		if derived != "" && derived != namespace {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"install: WARNING: declared namespace %q differs from origin-derived %q (fork? keeping declared)\n",
				namespace, derived)
		}
	}

	// Get the current commit (tolerate absence).
	commit, _ := r.R.Output(repoRoot, "rev-parse", "--short", "HEAD")
	commit = strings.TrimSpace(commit)

	// Delegate to the registry.
	reg := &registry.Registry{P: r.P}
	if err := reg.Install(repoRoot, namespace, url, commit, ff); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "installed %s (%s)\n",
		namespace, filepath.Base(repoRoot))
	return nil
}

// deriveNamespace converts a git remote URL to a reverse-DNS namespace.
// Handles ssh (git@host:path.git), https, and git:// forms.
// Returns "" when the URL cannot be parsed into a valid namespace.
func deriveNamespace(rawURL string) string {
	// Strip scheme prefixes.
	url := rawURL
	for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
		if strings.HasPrefix(url, prefix) {
			url = url[len(prefix):]
			break
		}
	}
	// Strip git@ prefix (SSH shorthand: git@host:path).
	if idx := strings.Index(url, "@"); idx >= 0 {
		url = url[idx+1:]
		// Replace : separator between host and path with /.
		url = strings.Replace(url, ":", "/", 1)
	}

	// Strip port (host:port/path → host/path already done above).
	// Strip trailing .git.
	url = strings.TrimSuffix(url, ".git")

	// Split into host + path segments.
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return ""
	}
	host := parts[0]
	// Strip user:pass@ from host if present (rare in remote URLs but possible).
	if idx := strings.LastIndex(host, "@"); idx >= 0 {
		host = host[idx+1:]
	}

	// Reverse host labels.
	hostParts := strings.Split(host, ".")
	for i, j := 0, len(hostParts)-1; i < j; i, j = i+1, j-1 {
		hostParts[i], hostParts[j] = hostParts[j], hostParts[i]
	}
	ns := strings.Join(hostParts, ".")

	// Append path segments (/ → .).
	if len(parts) == 2 && parts[1] != "" {
		pathSegs := strings.Split(parts[1], "/")
		for _, seg := range pathSegs {
			if seg != "" {
				ns += "." + seg
			}
		}
	}

	return strings.ToLower(ns)
}
