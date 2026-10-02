package cmd

import (
	"github.com/spf13/cobra"
)

func newOverrideCommand(opt *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "override",
		Short: "Run a repository's services from another checkout of it",
		Long: "An override points every managed service of one repository at another checkout of\n" +
			"that repository, normally a Git worktree, for the active runtime. Affected running\n" +
			"workspace services restart through rungrid start; nothing else is touched. Overrides\n" +
			"survive resume and end with down.",
		Example: "  rungrid override set api GH-12\n  rungrid override set api ~/worktrees/acme/api/GH-12\n" +
			"  rungrid override list\n  rungrid override clear api\n  rungrid override sync",
		Args: cobra.NoArgs,
	}
	command.AddCommand(
		&cobra.Command{
			Use:   "set <repository|service> <path|worktree>",
			Short: "Run a repository's managed services from another checkout",
			Long: "The checkout is a path, the branch or directory name of a registered worktree, or\n" +
				"primary to restore the original checkout. A service name selects its repository.",
			Example: "  rungrid override set api GH-12\n  rungrid override set worker ../api-fix",
			Args:    cobra.ExactArgs(2),
			RunE: func(command *cobra.Command, args []string) error {
				return runOverrideSet(command, opt, args[0], args[1])
			},
		},
		&cobra.Command{
			Use:     "clear [repository|service]",
			Short:   "Restore one repository, or every repository, to its declared checkout",
			Example: "  rungrid override clear api\n  rungrid override clear",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(command *cobra.Command, args []string) error {
				target := ""
				if len(args) == 1 {
					target = args[0]
				}
				return runOverrideClear(command, opt, target)
			},
		},
		&cobra.Command{
			Use:     "list",
			Short:   "List active overrides with path, branch, HEAD, and dirty state",
			Example: "  rungrid override list\n  rungrid override list --json",
			Args:    cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				return runOverrideList(command, opt)
			},
		},
		&cobra.Command{
			Use:   "sync",
			Short: "Apply the manifest's service worktree declarations to the runtime",
			Long: "Sync makes the active overrides equal the worktree: declarations in the manifest\n" +
				"and local overlay. Declared repositories are set; every other override is cleared.",
			Example: "  rungrid override sync",
			Args:    cobra.NoArgs,
			RunE: func(command *cobra.Command, _ []string) error {
				return runOverrideSync(command, opt)
			},
		},
	)
	return command
}
