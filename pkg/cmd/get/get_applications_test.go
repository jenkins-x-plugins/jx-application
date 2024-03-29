package get

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	extv1beta "k8s.io/api/extensions/v1beta1"

	fake2 "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"

	"github.com/jenkins-x-plugins/jx-application/pkg/applications"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
)

func TestGetApplicationsOptions_generateTable(t *testing.T) {
	jxclient := fake2.NewSimpleClientset()

	tests := []struct {
		name string
		want table.Table
	}{
		{name: "check_application_names", want: table.Table{Rows: [][]string{
			{"APPLICATION", "STAGING", "PODS", "URL", "PRODUCTION", "PODS", "URL"},
			{"testapp4", "1.0.3", "1/1", "http://testapp4-jx-staging.test.nip.io", "1.0.3", "1/1", "http://testapp4-jx-production.test.nip.io"},
			{"testapp5", "1.0.0", "1/1", "http://testapp5-jx-staging.test.nip.io", "", "", ""},
			{"testapp6", "1.0.1", "1/1", "http://testapp6-jx-staging.test.nip.io", "", "", ""},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			kubeclient := fake.NewSimpleClientset()
			loadTestIngresses(t, tt.name, kubeclient)

			applications := loadTestApplicationsList(t, tt.name)
			o := &ApplicationsOptions{
				KubeClient: kubeclient,
				JXClient:   jxclient,
			}
			if got := o.generateTable(applications); !reflect.DeepEqual(got.Rows, tt.want.Rows) {
				got.Render()
				t.Errorf("generateTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

// load test ingresses used to find a URL to display in the table
func loadTestIngresses(t *testing.T, name string, kubeclient *fake.Clientset) {
	file := filepath.Join("test_data", "generate_table", name, "ingresses.yaml")

	setupData, err := os.ReadFile(file)
	assert.NoError(t, err, "failed to read file")

	ingresses := &extv1beta.IngressList{}

	err = yaml.Unmarshal(setupData, ingresses)
	assert.NoError(t, err, "failed to unmarshal applications yaml")
	for i := range ingresses.Items {
		_, err := kubeclient.ExtensionsV1beta1().Ingresses(ingresses.Items[i].Namespace).Create(context.TODO(), &ingresses.Items[i], metav1.CreateOptions{})
		assert.NoError(t, err, "failed to create test ingress resource")
	}
}

// load the test applications
func loadTestApplicationsList(t *testing.T, name string) applications.List {
	file := filepath.Join("test_data", "generate_table", name, "applications.yaml")

	setupData, err := os.ReadFile(file)
	assert.NoError(t, err, "failed to read file")

	applications := &applications.List{}

	err = yaml.Unmarshal(setupData, applications)
	assert.NoError(t, err, "failed to unmarshal applications yaml")

	rs := *applications
	return rs
}
