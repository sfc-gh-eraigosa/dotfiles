package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage tmux-mgr shell aliases",
}

func init() {
	aliasCmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Generate and install shell aliases for tmux-mgr",
		Run: func(cmd *cobra.Command, args []string) {
			aliases := `
# tmux-mgr aliases
alias tmux-a='tmux-mgr session attach'
alias tmux-ls='tmux-mgr session list'
alias tmux-new='tmux-mgr session new'
alias tmux-kill='tmux-mgr session kill'
alias tmux-start='tmux-mgr session new antigravity -a'
`
			home, _ := os.UserHomeDir()
			configDir := filepath.Join(home, ".config", "tmux-mgr")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				fmt.Printf("Error creating %s: %v\n", configDir, err)
				return
			}

			aliasFile := filepath.Join(configDir, "aliases.sh")
			err := os.WriteFile(aliasFile, []byte(aliases), 0644)
			if err != nil {
				fmt.Printf("Error writing aliases file: %v\n", err)
				return
			}

			fmt.Printf("Aliases written to %s\n", aliasFile)

			// Generate completions
			completionDir := filepath.Join(configDir, "completions")
			if err := os.MkdirAll(completionDir, 0755); err != nil {
				fmt.Printf("Error creating %s: %v\n", completionDir, err)
				return
			}

			bashComp := filepath.Join(completionDir, "tmux-mgr.bash")
			if err := rootCmd.GenBashCompletionFile(bashComp); err != nil {
				fmt.Printf("Error writing bash completions: %v\n", err)
			}
			zshComp := filepath.Join(completionDir, "tmux-mgr.zsh")
			if err := rootCmd.GenZshCompletionFile(zshComp); err != nil {
				fmt.Printf("Error writing zsh completions: %v\n", err)
			}

			// Update aliases file to include completions and alias completions
			completionSource := `
# tmux-mgr completions
if [ -n "$BASH_VERSION" ]; then
    [ -f ${HOME}/.config/tmux-mgr/completions/tmux-mgr.bash ] && source ${HOME}/.config/tmux-mgr/completions/tmux-mgr.bash
elif [ -n "$ZSH_VERSION" ]; then
    [ -f ${HOME}/.config/tmux-mgr/completions/tmux-mgr.zsh ] && source ${HOME}/.config/tmux-mgr/completions/tmux-mgr.zsh
fi

# Alias completions
if command -v compdef >/dev/null 2>&1; then
    compdef _tmux-mgr tmux-a=session-attach
    compdef _tmux-mgr tmux-ls=session-list
    compdef _tmux-mgr tmux-new=session-new
    compdef _tmux-mgr tmux-kill=session-kill
fi
`

			// The error here used to be discarded and f used unconditionally: if
			// OpenFile failed, f was nil and f.WriteString panicked.
			cf, err := os.OpenFile(aliasFile, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Printf("Error appending completions to %s: %v\n", aliasFile, err)
				return
			}
			if _, err := cf.WriteString(completionSource); err != nil {
				fmt.Printf("Error writing completions to %s: %v\n", aliasFile, err)
			}
			if err := cf.Close(); err != nil {
				fmt.Printf("Error closing %s: %v\n", aliasFile, err)
			}

			sourceLine := "\n# Added by tmux-mgr\n[ -f ${HOME}/.config/tmux-mgr/aliases.sh ] && source ${HOME}/.config/tmux-mgr/aliases.sh\n"

			shellConfigs := []string{".zshrc", ".bashrc"}
			for _, cfg := range shellConfigs {
				cfgPath := filepath.Join(home, cfg)
				if _, err := os.Stat(cfgPath); err == nil {
					content, _ := os.ReadFile(cfgPath)
					if !contains(string(content), "tmux-mgr/aliases.sh") {
						f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0644)
						if err == nil {
							if _, err := f.WriteString(sourceLine); err != nil {
								fmt.Printf("Error updating %s: %v\n", cfg, err)
							}
							if err := f.Close(); err != nil {
								fmt.Printf("Error closing %s: %v\n", cfg, err)
							}
							fmt.Printf("Updated %s to source aliases\n", cfg)
						}
					} else {
						fmt.Printf("%s already sources aliases\n", cfg)
					}
				}
			}
		},
	})
	rootCmd.AddCommand(aliasCmd)
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
