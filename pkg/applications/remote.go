package applications

import (
	"path/filepath"

	"github.com/jenkins-x-plugins/jx-gitops/pkg/releasereport"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/pkg/errors"
)

// GetRemoteDeployments finds the remote cluster's
func GetRemoteDeployments(g gitclient.Interface, env *v1.Environment) (map[string]Deployment, error) {
	gitURL := env.Spec.Source.URL

	if gitURL == "" {
		return nil, errors.Errorf("no git URL on environment %s", env.Name)
	}

	dir, err := gitclient.CloneToDir(g, gitURL, "")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clone git URL %s for environment %s", gitURL, env.Name)
	}

	path := filepath.Join(dir, "docs", "releases.yaml")
	exists, err := files.FileExists(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check for file %s in git clone of %s", path, gitURL)
	}
	if !exists {
		return nil, nil
	}

	var releases []*releasereport.NamespaceReleases

	err = releasereport.LoadReleases(path, &releases)
	if err != nil {
		return nil, err
	}

	ns := env.Spec.Namespace
	for _, r := range releases {
		if r.Namespace == ns {
			return ToDeploymentMap(r.Releases), nil
		}
	}
	return nil, nil

}

func ToDeploymentMap(releases []*releasereport.ReleaseInfo) map[string]Deployment {
	m := map[string]Deployment{}
	for _, r := range releases {
		name := r.Name
		if name == "" {
			continue
		}
		m[name] = Deployment{
			Name:    name,
			URL:     r.ApplicationURL,
			Version: r.Version,
		}
	}
	return m
}
