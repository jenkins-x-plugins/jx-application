package applications

import (
	"context"
	"strconv"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"

	v1 "github.com/jenkins-x/jx-api/v3/pkg/apis/jenkins.io/v1"
	jxc "github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/services"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RevisionLabel the label used to show the revision
const RevisionLabel = "serving.knative.dev/revision"

// Deployment represents an application deployment in a single environment
type Deployment struct {
	*appsv1.Deployment `json:"deployment,omitempty"`
}

// Environment represents an environment in which an application has been
// deployed
type Environment struct {
	v1.Environment `json:"environment,omitempty"`
	Deployments    []Deployment `json:"deployments,omitempty"`
}

// Application represents an application in jx
type Application struct {
	*v1.SourceRepository `json:"sourceRepository"`
	Environments         map[string]Environment `json:"environments"`
}

// List is a collection of applications
type List struct {
	Items []Application `json:"applications,omitempty"`
}

// IsPreview returns true if the environment is a preview environment
func (e *Environment) IsPreview() bool {
	return e.Environment.Spec.Kind == v1.EnvironmentKindTypePreview
}

// Environments loops through all applications in a list and returns a map with
// all the unique environments
func (l List) Environments() map[string]v1.Environment {
	envs := make(map[string]v1.Environment)

	for _, a := range l.Items {
		for name, env := range a.Environments { //nolint
			if _, ok := envs[name]; !ok {
				envs[name] = env.Environment
			}
		}
	}

	return envs
}

// Name returns the application name
func (a Application) Name() string {
	return naming.ToValidName(a.SourceRepository.Spec.Repo)
}

// Version returns the deployment version
func (d Deployment) Version() string {
	return getVersion(&d.Deployment.ObjectMeta)
}

// getVersion returns the version from the labels on the deployment if it can be deduced
func getVersion(r *metav1.ObjectMeta) string {
	if r != nil {
		labels := r.Labels
		if labels != nil {
			v := labels["version"]
			if v != "" {
				return v
			}
			v = labels["chart"]
			if v != "" {
				arr := strings.Split(v, "-")
				last := arr[len(arr)-1]
				if last != "" {
					return last
				}
				return v
			}

			// find the kserve revision
			kversion := labels[RevisionLabel]
			if kversion != "" {
				idx := strings.LastIndex(kversion, "-")
				if idx > 0 {
					kversion = kversion[idx+1:]
				}
				return kversion
			}
		}
	}
	return ""
}

// Pods returns the ratio of pods that are ready/replicas
func (d Deployment) Pods() string {
	pods := ""
	ready := d.Deployment.Status.ReadyReplicas

	if d.Deployment.Spec.Replicas != nil && ready > 0 {
		replicas := int32ToA(*d.Deployment.Spec.Replicas)
		strconv.FormatInt(int64(*d.Deployment.Spec.Replicas), 10)
		pods = int32ToA(ready) + "/" + replicas
	}

	return pods
}

func int32ToA(n int32) string {
	return strconv.FormatInt(int64(n), 10)
}

// URL returns a deployment URL
func (d Deployment) URL(kc kubernetes.Interface, a Application) string {
	url, _ := services.FindServiceURL(kc, d.Deployment.Namespace, a.Name())
	return url
}

// GetApplications fetches all Applications
func GetApplications(jxClient jxc.Interface, kubeClient kubernetes.Interface, namespace string) (List, error) {
	list := List{
		Items: make([]Application, 0),
	}

	// fetch ALL repositories
	srList, err := jxClient.JenkinsV1().SourceRepositories(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return list, errors.Wrapf(err, "failed to find any SourceRepositories in namespace %s", namespace)
	}

	// fetch all environments
	envMap, _, err := jxenv.GetOrderedEnvironments(jxClient, namespace)
	if err != nil {
		return list, errors.Wrapf(err, "failed to fetch environments in namespace %s", namespace)
	}

	// only keep permanent environments
	permanentEnvsMap := map[string]*v1.Environment{}
	for _, env := range envMap {
		if env.Spec.Kind.IsPermanent() {
			permanentEnvsMap[env.Spec.Namespace] = env
		}
	}

	// copy repositories that aren't environments to our applications list
	for i := range srList.Items {
		srCopy := srList.Items[i]
		if !jxenv.IsIncludedInTheGivenEnvs(permanentEnvsMap, &srCopy) {
			list.Items = append(list.Items, Application{&srCopy, make(map[string]Environment)})
		}
	}

	// fetch deployments by environment (excluding dev)
	deployments := make(map[string]map[string]appsv1.Deployment)
	for _, env := range permanentEnvsMap {
		if env.Spec.Kind != v1.EnvironmentKindTypeDevelopment {
			var envDeployments map[string]appsv1.Deployment
			envDeployments, err = getDeployments(kubeClient, env.Spec.Namespace)
			if err != nil {
				return list, err
			}
			deployments[env.Spec.Namespace] = envDeployments
		}
	}

	err = list.appendMatchingDeployments(permanentEnvsMap, deployments)
	if err != nil {
		return list, err
	}

	return list, nil
}

func getDeploymentAppNameInEnvironment(d *appsv1.Deployment, e *v1.Environment) (string, error) {
	labels, err := metav1.LabelSelectorAsMap(d.Spec.Selector)
	if err != nil {
		return "", err
	}

	name := GetAppName(labels["app"], e.Spec.Namespace)
	return name, nil
}

func (l List) appendMatchingDeployments(envs map[string]*v1.Environment, deps map[string]map[string]appsv1.Deployment) error {
	for _, app := range l.Items {
		for envName, env := range envs {
			for i := range deps[envName] {
				dep := deps[envName][i]
				depAppName, err := getDeploymentAppNameInEnvironment(&dep, env)
				if err != nil {
					return errors.Wrap(err, "getting app name")
				}
				if depAppName == app.Name() && !isCanaryAuxiliaryDeployment(&dep) {
					depCopy := dep
					app.Environments[env.Name] = Environment{
						*env,
						[]Deployment{{&depCopy}},
					}
				}
			}
		}
	}

	return nil
}

// IsCanaryAuxiliaryDeployment returns whether this deployment has been created automatically by Flagger from a Canary object
func isCanaryAuxiliaryDeployment(d *appsv1.Deployment) bool {
	ownerReferences := d.GetObjectMeta().GetOwnerReferences()
	for i := range ownerReferences {
		if ownerReferences[i].Kind == "Canary" {
			return true
		}
	}
	return false
}

func GetAppName(name string, namespaces ...string) string {
	if name != "" {
		for _, ns := range namespaces {
			// for helm deployments which prefix the namespace in the name lets strip it
			prefix := ns + "-"
			name = strings.TrimPrefix(name, prefix)
		}

		// we often have the app name repeated twice - particularly when using helm 3
		l := len(name) / 2
		if name[l] == '-' {
			first := name[0:l]
			if name[l+1:] == first {
				return first
			}
		}

		// The applications seems to be prefixed with jx regardless of the namespace
		// where they are deployed. Let's remove this prefix.
		prefix := "jx-"
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

// getDeployments get deployments in the given namespace
func getDeployments(kubeClient kubernetes.Interface, ns string) (map[string]appsv1.Deployment, error) {
	answer := map[string]appsv1.Deployment{}
	deps, err := kubeClient.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return answer, err
	}
	for i := range deps.Items {
		d := deps.Items[i]
		answer[d.Name] = d
	}
	return answer, nil
}

func GetEditAppName(name string) string {
	// we often have the app name repeated twice!
	l := len(name) / 2
	if name[l] == '-' {
		first := name[0:l]
		if name[l+1:] == first {
			return first
		}
	}
	return name
}
