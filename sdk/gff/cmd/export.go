package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	exportFormat string
	exportOutput string
	exportShell  bool
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export feature flags in shell, dotenv, json, or yaml format",
	Long: `Export writes feature flag values as environment variables or structured data.

Formats:
  shell   eval-able GFF_<MANGLED>=<value> lines (stdout)
  dotenv  identical KEY=value lines; default -o .env
  json    full resolved snapshot as []ResolvedJSON (stdout)
  yaml    same snapshot in YAML encoding (stdout)

Values are bool literals (true|false) or comma-joined kebab option ids.`,
	Args: cobra.NoArgs,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "shell", "output format: shell|dotenv|json|yaml")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "write output to file (dotenv defaults to .env when -o omitted)")
	exportCmd.Flags().BoolVar(&exportShell, "shell", false, "alias for --format shell")
	rootCmd.AddCommand(exportCmd)
}

// resetExportFlags resets the export-verb package-level flags to their defaults.
// Tests call this via t.Cleanup so each runCmd call starts clean.
func resetExportFlags() {
	exportFormat = "shell"
	exportOutput = ""
	exportShell = false
}

// mangleKey converts a gff key to its env-var name:
// uppercase, replacing '.' and '-' with '_', prefixed with GFF_.
func mangleKey(key string) string {
	r := strings.NewReplacer(".", "_", "-", "_")
	return "GFF_" + strings.ToUpper(r.Replace(key))
}

func runExport(cmd *cobra.Command, _ []string) error {
	// --shell is an alias for --format shell.
	if exportShell {
		exportFormat = "shell"
	}

	r, err := newResolver()
	if err != nil {
		return err
	}

	all, err := r.All()
	if err != nil {
		return err
	}

	switch exportFormat {
	case "shell", "dotenv":
		return exportKV(cmd, all)
	case "json":
		return exportJSON(cmd, all)
	case "yaml":
		return exportYAML(cmd, all)
	default:
		return fmt.Errorf("export: unknown format %q; want shell|dotenv|json|yaml", exportFormat)
	}
}

// kvLine renders one flag as a KEY=value line.
func kvLine(res resolve.Resolved) string {
	var value string
	switch v := res.Value.GetKind().(type) {
	case *gffv1.Value_BoolValue:
		if v.BoolValue {
			value = "true"
		} else {
			value = "false"
		}
	case *gffv1.Value_ChoiceValue:
		value = strings.Join(v.ChoiceValue.GetSelected(), ",")
	}
	return mangleKey(res.Feature.GetPath()) + "=" + value
}

// exportKV writes KEY=value lines for shell and dotenv formats.
// Lines are sorted by key for deterministic output.
func exportKV(cmd *cobra.Command, all []resolve.Resolved) error {
	lines := make([]string, 0, len(all))
	for _, res := range all {
		lines = append(lines, kvLine(res))
	}
	sort.Strings(lines)

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}

	return writeOutput(cmd, content)
}

// exportJSON writes a JSON array of ResolvedJSON to stdout or -o file.
func exportJSON(cmd *cobra.Command, all []resolve.Resolved) error {
	rjs := make([]resolve.ResolvedJSON, 0, len(all))
	for _, res := range all {
		rj, err := res.JSON()
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		rjs = append(rjs, rj)
	}

	b, err := json.Marshal(rjs)
	if err != nil {
		return fmt.Errorf("export: json marshal: %w", err)
	}

	return writeOutput(cmd, string(b)+"\n")
}

// exportYAML writes a YAML-encoded list of ResolvedJSON to stdout or -o file.
func exportYAML(cmd *cobra.Command, all []resolve.Resolved) error {
	// Build []ResolvedJSON then re-encode through JSON to get plain maps (no
	// protojson oneofs leaking into the yaml encoder).
	rjs := make([]resolve.ResolvedJSON, 0, len(all))
	for _, res := range all {
		rj, err := res.JSON()
		if err != nil {
			return fmt.Errorf("export: %w", err)
		}
		rjs = append(rjs, rj)
	}

	// JSON → []any → YAML (the protojson-normalize trick).
	jsonBytes, err := json.Marshal(rjs)
	if err != nil {
		return fmt.Errorf("export: json marshal for yaml: %w", err)
	}
	var intermediate any
	if err := json.Unmarshal(jsonBytes, &intermediate); err != nil {
		return fmt.Errorf("export: json unmarshal for yaml: %w", err)
	}
	yamlBytes, err := yaml.Marshal(intermediate)
	if err != nil {
		return fmt.Errorf("export: yaml marshal: %w", err)
	}

	return writeOutput(cmd, string(yamlBytes))
}

// writeOutput writes content to -o file (atomic, no partial write) or stdout.
func writeOutput(cmd *cobra.Command, content string) error {
	outPath := exportOutput
	// dotenv default target.
	if exportFormat == "dotenv" && outPath == "" {
		outPath = ".env"
	}

	if outPath == "" {
		_, err := io.WriteString(cmd.OutOrStdout(), content)
		return err
	}

	// Resolve to absolute path.
	if abs, err := filepath.Abs(outPath); err == nil {
		outPath = abs
	}

	// Atomic write: temp file in same dir + rename.
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("export: mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".gff-export-*.tmp")
	if err != nil {
		return fmt.Errorf("export: create temp: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.WriteString(tmp, content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("export: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("export: close temp: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("export: rename to %s: %w", outPath, err)
	}
	success = true
	return nil
}
