module github.com/jenkins-x/jx-application

require (
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/ghodss/yaml v1.0.0
	github.com/jenkins-x/jx-api/v4 v4.0.24
	github.com/jenkins-x/jx-helpers/v3 v3.0.75
	github.com/jenkins-x/jx-logging/v3 v3.0.3
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.20.3
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v11.0.0+incompatible

)

replace (
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2
)

go 1.15
