package get

import (
	"os"
	"sort"
	"strings"

	"github.com/jenkins-x/jx-application/pkg/applications"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"

	"github.com/jenkins-x/jx-helpers/v3/pkg/options"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"

	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/spf13/cobra"
	appsV1 "k8s.io/api/apps/v1"
)

// ApplicationsOptions containers the CLI options
type ApplicationsOptions struct {
	options.BaseOptions

	KubeClient kubernetes.Interface
	JXClient   jxc.Interface

	CurrentNamespace string
	Namespace        string
	Environment      string
	HideURL          bool
	HidePod          bool
}

// Applications is a map indexed by the application name then the environment name
// Applications map[string]map[string]*ApplicationEnvironmentInfo

// EnvApps contains data about app deployments in an environment
type EnvApps struct {
	Environment v1.Environment
	Apps        map[string]appsV1.Deployment
}

// ApplicationEnvironmentInfo contains the results of an app for an environment
type ApplicationEnvironmentInfo struct {
	Deployment  *appsV1.Deployment
	Environment *v1.Environment
	Version     string
	URL         string
}

var (
	getVersionLong = templates.LongDesc(`
		Display applications across environments.
`)

	getVersionExample = templates.Examples(`
		# List applications, their URL and pod counts for all environments
		jx get applications
		# List applications only in the Staging environment
		jx get applications -e staging
		# List applications only in the Production environment
		jx get applications -e production
		# List applications only in a specific namespace
		jx get applications -n jx-staging
		# List applications hiding the URLs
		jx get applications -u
		# List applications just showing the versions (hiding urls and pod counts)
		jx get applications -u -p
	`)
)

// NewCmdGetApplications creates the new command for: jx get version
func NewCmdGetApplications() (*cobra.Command, *ApplicationsOptions) {
	o := &ApplicationsOptions{}
	cmd := &cobra.Command{
		Use:     "get application",
		Short:   "Display one or more Applications and their versions",
		Aliases: []string{"applications", "apps"},
		Long:    getVersionLong,
		Example: getVersionExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&o.HideURL, "url", "u", false, "Hide the URLs")
	cmd.Flags().BoolVarP(&o.HidePod, "pod", "p", false, "Hide the pod counts")
	cmd.Flags().StringVarP(&o.Environment, "env", "e", "", "Filter applications in the given environment")
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "Filter applications in the given namespace")

	return cmd, o
}

// Validate verifies settings
func (o *ApplicationsOptions) Validate() error {
	var err error
	if o.JXClient == nil {
		o.JXClient, o.CurrentNamespace, err = jxclient.LazyCreateJXClientAndNamespace(o.JXClient, o.CurrentNamespace)
		if err != nil {
			return errors.Wrapf(err, "failed to create jx client")
		}
	}

	if o.KubeClient == nil {
		o.KubeClient, err = kube.LazyCreateKubeClient(o.KubeClient)
		if err != nil {
			return errors.Wrapf(err, "failed to create kube client")
		}
	}
	ns, _, err := jxenv.GetDevNamespace(o.KubeClient, o.CurrentNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to find dev namespace")
	}
	if ns != "" {
		o.CurrentNamespace = ns
	}
	return nil
}

// Run implements this command
func (o *ApplicationsOptions) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate")
	}
	err = o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate")
	}

	list, err := applications.GetApplications(o.JXClient, o.KubeClient, o.CurrentNamespace)
	if err != nil {
		return errors.Wrap(err, "fetching applications")
	}
	if len(list.Items) == 0 {
		log.Logger().Infof("No applications found")
		return nil
	}

	table := o.generateTable(list)
	table.Render()

	return nil
}

func (o *ApplicationsOptions) generateTable(list applications.List) table.Table {
	table := o.generateTableHeaders(list)

	for i := range list.Items {
		a := &list.Items[i]
		row := []string{}
		name := a.Name()
		environments := a.Environments
		if len(environments) > 0 {
			envMap := list.Environments()
			keys := o.sortedKeys(envMap)
			for _, k := range keys {
				if ae, ok := environments[k]; ok {
					for _, d := range ae.Deployments {
						name = applications.GetAppName(d.Deployment.Name, k)
						if ae.Environment.Spec.Kind == v1.EnvironmentKindTypeEdit {
							name = applications.GetEditAppName(name)
						} else if ae.Environment.Spec.Kind == v1.EnvironmentKindTypePreview {
							name = ae.Environment.Spec.PullRequestURL
						}
						if !ae.IsPreview() {
							row = append(row, d.Version())
						}
						if !o.HidePod {
							row = append(row, d.Pods())
						}
						if !o.HideURL {
							row = append(row, d.URL(o.KubeClient, a))
						}
					}
				} else {
					if !ae.IsPreview() {
						row = append(row, "")
					}
					if !o.HidePod {
						row = append(row, "")
					}
					if !o.HideURL {
						row = append(row, "")
					}
				}
			}
			row = append([]string{name}, row...)

			table.AddRow(row...)
		}
	}
	return table
}

func envTitleName(e v1.Environment) string { //nolint
	if e.Spec.Kind == v1.EnvironmentKindTypeEdit {
		return "Edit"
	}

	return e.Name
}

func (o *ApplicationsOptions) sortedKeys(envs map[string]v1.Environment) []string {
	keys := make([]string, 0, len(envs))
	for k, env := range envs { //nolint
		if (o.Environment == "" || o.Environment == k) && (o.Namespace == "" || o.Namespace == env.Spec.Namespace) {
			keys = append(keys, k)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	return keys
}

func (o *ApplicationsOptions) generateTableHeaders(list applications.List) table.Table {
	t := table.CreateTable(os.Stdout)
	title := "APPLICATION"
	titles := []string{title}

	envs := list.Environments()

	for _, k := range o.sortedKeys(envs) {
		titles = append(titles, strings.ToUpper(envTitleName(envs[k])))

		if !o.HidePod {
			titles = append(titles, "PODS")
		}
		if !o.HideURL {
			titles = append(titles, "URL")
		}
	}
	t.AddRow(titles...)
	return t
}
