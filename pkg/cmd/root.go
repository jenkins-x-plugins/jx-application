package cmd

import (
	"os"

	"github.com/jenkins-x/jx-application/pkg/cmd/version"
	"github.com/jenkins-x/jx-application/pkg/rootcmd"
	"github.com/jenkins-x/jx-helpers/pkg/cobras"
	"github.com/jenkins-x/jx/v2/pkg/cmd/clients"
	"github.com/jenkins-x/jx/v2/pkg/cmd/get"
	"github.com/jenkins-x/jx/v2/pkg/cmd/opts"
	"github.com/spf13/cobra"
)

// Main creates the new command
func Main() *cobra.Command {
	f := clients.NewFactory()
	commonOpts := opts.NewCommonOptionsWithTerm(f, os.Stdin, os.Stdout, os.Stderr)

	cmd := get.NewCmdGetApplications(commonOpts)

	cmd.Use = rootcmd.TopLevelCommand
	cmd.Short = "command for viewing deployed Applications across Environments"

	commonOpts.AddBaseFlags(cmd)

	cmd.AddCommand(cobras.SplitCommand(version.NewCmdVersion()))
	return cmd
}
