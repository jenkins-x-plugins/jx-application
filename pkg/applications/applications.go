package applications

import (
	"context"
	"strconv"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"

	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxenv"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	jxc "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/services"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// IsPreview returns true if the environment is a preview environment
func (e *Environment) IsPreview() bool {
	return e.Environment.Spec.Kind == v1.EnvironmentKindTypePreview
}

// Environments loops through all applications in a list and returns a map with
// all the unique environments
func (l *List) Environments() map[string]v1.Environment {
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
func (a *Application) Name() string {
	return naming.ToValidName(a.SourceRepository.Spec.Repo)
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
func Pods(d *appsv1.Deployment) string {
	pods := ""
	ready := d.Status.ReadyReplicas

	if d.Spec.Replicas != nil && ready > 0 {
		replicas := int32ToA(*d.Spec.Replicas)
		strconv.FormatInt(int64(*d.Spec.Replicas), 10)
		pods = int32ToA(ready) + "/" + replicas
	}

	return pods
}

func int32ToA(n int32) string {
	return strconv.FormatInt(int64(n), 10)
}

// DeploymentURL returns a deployment URL
func DeploymentURL(kc kubernetes.Interface, d *appsv1.Deployment, appName string) string {
	url, _ := services.FindServiceURL(kc, d.Namespace, appName)
	return url
}

// GetApplications fetches all Applications
func GetApplications(jxClient jxc.Interface, kubeClient kubernetes.Interface, namespace string, g gitclient.Interface) (List, error) {
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
	deployments := make(map[string]map[string]Deployment)
	for _, env := range permanentEnvsMap {
		if env.Spec.Kind != v1.EnvironmentKindTypeDevelopment {
			var envDeployments map[string]Deployment
			if env.Spec.RemoteCluster {
				envDeployments, err = GetRemoteDeployments(g, env)
				deployments[env.Spec.Namespace] = envDeployments
				if err != nil {
					return list, err
				}
				continue
			}

			envDeployments, err = getDeployments(kubeClient, env.Spec.Namespace, env)
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

func (l *List) appendMatchingDeployments(envs map[string]*v1.Environment, deps map[string]map[string]Deployment) error {
	for _, app := range l.Items {
		for envName, env := range envs {
			for i := range deps[envName] {
				dep := deps[envName][i]
				if dep.Name == app.Name() && !dep.Canary {
					app.Environments[env.Name] = Environment{
						*env,
						[]Deployment{dep},
					}
				}
			}
		}
	}

	return nil
}

func CreateDeployment(d *appsv1.Deployment, env *v1.Environment) (Deployment, error) {
	name := GetAppName(d.Name, d.Namespace)

	answer := Deployment{
		Name:    name,
		Pods:    Pods(d),
		Version: getVersion(&d.ObjectMeta),
		Canary:  isCanaryAuxiliaryDeployment(d),
	}
	depAppName, err := getDeploymentAppNameInEnvironment(d, env)
	if err != nil {
		return answer, errors.Wrap(err, "getting app name")
	}
	if depAppName != "" {
		answer.Name = depAppName
	}
	return answer, nil

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

		// The applications seems to be prefixed with jx regardless of the namespace
		// where they are deployed. Let's remove this prefix.
		prefix := "jx-"
		name = strings.TrimPrefix(name, prefix)

		// we often have the app name repeated twice - particularly when using helm 3
		l := len(name) / 2
		if name[l] == '-' {
			first := name[0:l]
			if name[l+1:] == first {
				return first
			}
		}
	}
	return name
}

// getDeployments get deployments in the given namespace
func getDeployments(kubeClient kubernetes.Interface, ns string, env *v1.Environment) (map[string]Deployment, error) {
	answer := map[string]Deployment{}
	deps, err := kubeClient.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return answer, err
	}
	for i := range deps.Items {
		d := &deps.Items[i]
		deployment, err := CreateDeployment(d, env)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create Deployment for %s in namespace %s", d.Name, ns)
		}
		deployment.URL = DeploymentURL(kubeClient, d, deployment.Name)
		answer[d.Name] = deployment
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
