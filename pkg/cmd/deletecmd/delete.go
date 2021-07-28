package deletecmd

import (
	"fmt"
	"github.com/jenkins-x-plugins/jx-promote/pkg/environments"
	"github.com/jenkins-x/go-scm/scm"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"os"

	"github.com/spf13/cobra"
)

// Options the flags for updating webhooks
type Options struct {
	options.BaseOptions
	environments.EnvironmentPullRequestOptions

	GitURL           string
	EnvironmentName  string
	AutoMerge        bool
	PullRequestTitle string
	PullRequestBody  string
	NoSourceConfig   bool
	Owner            string
	Repository       string
	RemoveNamespace  string
	Namespace        string
	KubeClient       kubernetes.Interface
	JXClient         jxc.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Deletes the application deployments and removes the lighthouse configuration

		This command actually create a Pull Request on the development cluster git repository so you can review the changes to be made.

`)

	cmdExample = templates.Examples(`
		# deletes the application with the given name from the development cluster
		jx application delete --name myapp

		# deletes the deployed application for the remote production cluster only
		jx application delete --name myapp --env production

		# deletes the application with the given name with the git owner 
		jx application delete --name myapp --owner myorg

		# deletes the deployed applications but doesn't remove the '.jx/gitops/source-config.yaml' entry - so new releases come back
		jx application delete --name myapp --owner myorg --no-source
`)
)

func NewCmdDelete() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Deletes the application deployments and removes the lighthouse configuration",
		Long:    cmdLong,
		Example: cmdExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.GitURL, "url", "u", "", "The git URL of the cluster git repository to modify")
	cmd.Flags().StringVarP(&o.EnvironmentName, "env", "e", "dev", "The Environment name used to find the repository git URL if none is specified")
	cmd.Flags().BoolVarP(&o.AutoMerge, "auto-merge", "", true, "should we automatically merge if the PR pipeline is green")
	cmd.Flags().StringVar(&o.PullRequestTitle, "pull-request-title", "", "the PR title")
	cmd.Flags().StringVar(&o.PullRequestBody, "pull-request-body", "", "the PR body")

	cmd.Flags().BoolVarP(&o.NoSourceConfig, "no-source", "", false, "Do not remove the repository from the '.jx/gitops/source-config/yaml' file - so that a new release will come back")

	o.EnvironmentPullRequestOptions.ScmClientFactory.AddFlags(cmd)

	eo := &o.EnvironmentPullRequestOptions
	cmd.Flags().StringVarP(&eo.CommitTitle, "commit-title", "", "", "the commit title")
	cmd.Flags().StringVarP(&eo.CommitMessage, "commit-message", "", "", "the commit message")

	cmd.Flags().StringVarP(&o.Owner, "owner", "o", "", "The name of the git organisation or user which owns the app")
	cmd.Flags().StringVarP(&o.Repository, "repo", "r", "", "The name of the repository to remove")
	cmd.Flags().StringVarP(&o.RemoveNamespace, "remove-ns", "", "", "The namespace to remove the app from. If blank remove from all deployed namespaces")

	o.BaseOptions.AddBaseFlags(cmd)

	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	var err error

	if o.Repository == "" {
		return options.MissingOption("name")
	}

	if o.GitURL == "" {
		o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to create kube client")
		}
		o.JXClient, err = jxclient.LazyCreateJXClient(o.JXClient)
		if err != nil {
			return errors.Wrapf(err, "failed to create jx client")
		}
		ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to find dev namespace in %s", o.Namespace)
		}
		if ns != "" {
			o.Namespace = ns
		}

		env, err := jxenv.GetEnvironment(o.JXClient, ns, o.EnvironmentName)
		if err != nil {
			return errors.Wrapf(err, "failed to find Environment %s in namespace %s", o.EnvironmentName, ns)
		}

		o.GitURL = env.Spec.Source.URL
		if o.GitURL == "" {
			return errors.Errorf("no git URL for Environment %s in namespace %s", o.EnvironmentName, ns)
		}
		if env.Spec.RemoteCluster {
			o.NoSourceConfig = true
		}
	}

	// lazy create git
	o.EnvironmentPullRequestOptions.Git()
	return nil
}

// Run runs the command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	if o.PullRequestTitle == "" {
		o.PullRequestTitle = fmt.Sprintf("fix: remove app " + o.AppDescription())
	}
	if o.CommitTitle == "" {
		o.CommitTitle = o.PullRequestTitle
	}
	source := ""
	details := &scm.PullRequest{
		Source: source,
		Title:  o.PullRequestTitle,
		Body:   o.PullRequestBody,
		Draft:  false,
	}

	for _, label := range o.Labels {
		details.Labels = append(details.Labels, &scm.Label{
			Name:        label,
			Description: label,
		})
	}

	o.Function = func() error {
		dir := o.OutDir
		return o.DeleteApp(dir)
	}

	_, err = o.EnvironmentPullRequestOptions.Create(o.GitURL, "", details, o.AutoMerge)
	if err != nil {
		return errors.Wrapf(err, "failed to create Pull Request on repository %s", o.GitURL)
	}
	return nil
}

// AppDescription returns the app description
func (o *Options) AppDescription() string {
	if o.Owner == "" {
		return o.Repository
	}
	return scm.Join(o.Owner, o.Repository)
}

func (o *Options) DeleteApp(dir string) error {
	if !o.NoSourceConfig {
		// lets remove the source config
		args := []string{"gitops", "repository", "delete", "--name", o.Repository}
		if o.Owner != "" {
			args = append(args, "--owner", o.Owner)
		}

		c := &cmdrunner.Command{
			Dir:  dir,
			Name: "jx",
			Args: args,
			Out:  os.Stdout,
			Err:  os.Stderr,
		}
		_, err := o.CommandRunner(c)
		if err != nil {
			return errors.Wrapf(err, "failed to invoke %s", c.CLI())
		}
	}

	// now lets remove the promoted charts
	args := []string{"gitops", "helmfile", "delete", "--chart", o.Repository}
	if o.RemoveNamespace != "" {
		args = append(args, "--namespace", o.RemoveNamespace)
	}

	c := &cmdrunner.Command{
		Dir:  dir,
		Name: "jx",
		Args: args,
		Out:  os.Stdout,
		Err:  os.Stderr,
	}
	_, err := o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to invoke %s", c.CLI())
	}
	return nil
}
