package cmd

import (
	"context"
	"os"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/lifecycle"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	Dirty     = "unknown"
)

type options struct {
	configPath string
	localPath  string
	stateDir   string
	projectID  string
	json       bool
	noColor    bool
	quiet      bool
	verbose    bool
}

func Execute() error {
	root := newRootCommand()
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.Execute()
}

func ExitCode(err error) int { return errs.Code(err) }

func newRootCommand() *cobra.Command {
	opt := &options{}
	root := &cobra.Command{
		Use:           "rungrid",
		Short:         "Run a reproducible local development workspace",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	flags := root.PersistentFlags()
	flags.StringVar(&opt.configPath, "config", ".rungrid.yaml", "manifest path")
	flags.StringVar(&opt.localPath, "local", "", "local overlay path")
	flags.StringVar(&opt.stateDir, "state-dir", "", "state root override")
	flags.StringVar(&opt.projectID, "project", "", "select a known project id")
	flags.BoolVar(&opt.json, "json", false, "emit rungrid/output/v1 JSON")
	flags.BoolVar(&opt.noColor, "no-color", false, "disable ANSI color")
	flags.BoolVarP(&opt.quiet, "quiet", "q", false, "suppress non-error output")
	flags.BoolVarP(&opt.verbose, "verbose", "v", false, "include redacted diagnostics")

	root.AddCommand(
		newInitCommand(opt),
		newInstructionsCommand(opt),
		newDoctorCommand(opt),
		newPlanCommand(opt),
		newGenerateCommand(opt),
		newUpCommand(opt),
		newOpenCommand(opt),
		newAttachCommand(opt),
		newVersionsCommand(opt),
		newStatusCommand(opt),
		newLogsCommand(opt),
		newSyncCommand(opt),
		newReconcileCommand(opt),
		newWorktreesCommand(opt),
		newSessionCommand(opt),
		newStartCommand(opt),
		newStopCommand(opt),
		newDownCommand(opt),
		newUninstallCommand(opt),
		newConfigCommand(opt),
		newCompletionCommand(root),
		newVersionCommand(opt),
		newInternalCommand(opt),
	)
	configureHelp(root, opt)
	return root
}

func (o *options) load() (*manifest.Loaded, error) {
	return manifest.Load(o.configPath, o.localPath)
}

func (o *options) active(ctx context.Context) (lifecycle.Active, error) {
	projectID := o.projectID
	if projectID == "" {
		loaded, err := o.load()
		if err != nil {
			return lifecycle.Active{}, err
		}
		projectID = loaded.Manifest.Project.ID
	}
	return lifecycle.LoadActive(ctx, projectID, o.stateDir)
}
