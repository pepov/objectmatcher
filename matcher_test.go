package objectmatcher

import (
	"testing"

	"github.com/banzaicloud/objectmatcher/pkg/apply"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSimple(t *testing.T) {
	if !*integration {
		t.Skip()
	}

	original := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:               "test-",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "http",
					Port:       80,
				},
			},
			Selector: map[string]string{
				"app": "test",
			},
			Type: v1.ServiceTypeLoadBalancer,
		},
	}

	modified := original.DeepCopy()

	err := apply.CreateApplyAnnotation(original)
	if err != nil {
		t.Fatalf("failed to create apply annotation: %s", err.Error())
	}

	current, err := testContext.Client.CoreV1().Services(testContext.Namespace).Create(original)
	if err != nil {
		t.Fatalf("failed to create original: %s", err)
	}

	result, err := CalculatePatch(current, modified)
	if err != nil {
		t.Fatalf("failed to calculate patch: %s", err)
	}

	if !result.Unmodified() {
		t.Fatalf("Service should be matching with itself, however the diff: %v", result)
	}
}