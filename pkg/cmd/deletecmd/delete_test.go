package deletecmd_test

import (
	"github.com/jenkins-x-plugins/jx-application/pkg/cmd/deletecmd"
	"github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = true
)

func TestDelete(t *testing.T) {
	runner := &fakerunner.FakeRunner{
		CommandRunner: func(c *cmdrunner.Command) (string, error) {
			if c.Name == "git" && len(c.Args) > 0 && c.Args[0] == "push" {
				t.Logf("faking command %s in dir %s\n", c.CLI(), c.Dir)
				return "", nil
			}

			// lets really git clone but then fake out all other commands
			return cmdrunner.DefaultCommandRunner(c)
		},
	}
	scmClient, fakeData := fake.NewDefault()

	_, o := deletecmd.NewCmdDelete()
	o.Repository = "tekton-pipeline"
	o.GitURL = "https://github.com/jx3-gitops-repositories/jx3-kubernetes"
	o.CommandRunner = runner.Run
	o.ScmClient = scmClient

	// lets avoid creating a real scm client
	o.ScmClientFactory.ScmClient = scmClient
	o.ScmClientFactory.GitServerURL = giturl.GitHubURL

	err := o.Run()
	require.NoError(t, err, "failed to create app delete PR")

	require.Len(t, fakeData.PullRequests, 1, "should have 1 Pull Request created")

	for n, pr := range fakeData.PullRequests {
		t.Logf("created PR #%d with title: %s\n", n, pr.Title)
		t.Logf("body: %s\n\n", pr.Body)
	}
}
