package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage tmux sessions",
}

func init() {
	sessionCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all tmux sessions",
		Run: func(cmd *cobra.Command, args []string) {
			out, _ := Tmgr.Run("ls")
			fmt.Print(out)
		},
	})

	newSessionCmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create a new tmux session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			attach, _ := cmd.Flags().GetBool("attach")

			_, err := Tmgr.Run("new-session", "-d", "-s", name)
			if err != nil {
				log.Printf("Error creating session %s: %v", name, err)
				fmt.Printf("Error creating session %s\n", name)
				return
			}
			log.Printf("Session %s created", name)
			fmt.Printf("Session %s created\n", name)

			if attach {
				if err := Tmgr.Attach(name); err != nil {
					log.Printf("Error attaching to session %s: %v", name, err)
					fmt.Printf("Error attaching to session %s\n", name)
				}
			}
		},
	}
	newSessionCmd.Flags().BoolP("attach", "a", false, "Attach to the session after creation")
	sessionCmd.AddCommand(newSessionCmd)

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "attach [name]",
		Short: "Attach to a tmux session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			if err := Tmgr.Attach(name); err != nil {
				log.Printf("Error attaching to session %s: %v", name, err)
				fmt.Printf("Error attaching to session %s\n", name)
			}
		},
	})

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "detach",
		Short: "Detach from the current tmux session",
		Run: func(cmd *cobra.Command, args []string) {
			if !runTmux("detach-client") {
				return
			}
			log.Printf("Detached from session")
		},
	})

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "kill [name]",
		Short: "Kill a tmux session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			_, err := Tmgr.Run("kill-session", "-t", name)
			if err != nil {
				log.Printf("Error killing session %s: %v", name, err)
				fmt.Printf("Error killing session %s\n", name)
				return
			}
			log.Printf("Session %s killed", name)
			fmt.Printf("Session %s killed\n", name)
		},
	})

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "save [name]",
		Short: "Save current session layout",
		Run: func(cmd *cobra.Command, args []string) {
			name := "default"
			if len(args) > 0 {
				name = args[0]
			}
			if err := Tmgr.SaveLayout(name); err != nil {
				log.Printf("Error saving session: %v", err)
				fmt.Printf("Error saving session %s\n", name)
				return
			}
			fmt.Printf("Session %s saved\n", name)
		},
	})

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "restore [name]",
		Short: "Restore a saved session layout",
		Run: func(cmd *cobra.Command, args []string) {
			name := "default"
			if len(args) > 0 {
				name = args[0]
			}
			if err := Tmgr.RestoreLayout(name); err != nil {
				log.Printf("Error restoring session: %v", err)
				fmt.Printf("Error restoring session %s\n", name)
				return
			}
			fmt.Printf("Session %s restored\n", name)
		},
	})

	rootCmd.AddCommand(sessionCmd)
}
