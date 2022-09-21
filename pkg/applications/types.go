package applications

import v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"

// RevisionLabel the label used to show the revision
const RevisionLabel = "serving.knative.dev/revision"

// Deployment represents an application deployment in a single environment
type Deployment struct {
	Name    string `json:"name,omitempty"`
	Pods    string `json:"pods,omitempty"`
	Version string `json:"version,omitempty"`
	URL     string `json:"url,omitempty"`
	Canary  bool   `json:"canary,omitempty"`
	// *appsv1.Deployment `json:"deployment,omitempty"`
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
