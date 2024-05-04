package engine

import (
	"testing"

	kyvernov1 "github.com/kyverno/kyverno/api/kyverno/v1"
	engineapi "github.com/kyverno/kyverno/pkg/engine/api"
	"github.com/kyverno/kyverno/pkg/engine/policycontext"
	"github.com/kyverno/kyverno/pkg/logging"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_engine_filterRule(t *testing.T) {
	const (
		testRuleName = `test-rule`
	)
	logger := logging.WithName("test-logger")

	type fields struct {
		client engineapi.Client
	}

	type policyContextFields struct {
		policy      kyvernov1.Policy
		newResource unstructured.Unstructured
	}

	type args struct {
		rule          kyvernov1.Rule
		policyContext policyContextFields
	}

	type testCase struct {
		name             string
		fields           fields
		args             args
		expectedRuleResp *engineapi.RuleResponse
	}

	tests := []testCase{
		{
			fields: fields{},
			args: args{
				rule: kyvernov1.Rule{
					Name: testRuleName,
					Generation: kyvernov1.Generation{
						ResourceSpec: kyvernov1.ResourceSpec{
							APIVersion: "apps/v1",
						},
					},
				},
				policyContext: policyContextFields{
					policy: kyvernov1.Policy{},
					newResource: convertToUnstructured(t, &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test",
						},
					}),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			egn := &engine{
				client: tt.fields.client,
			}

			policyContext := &policycontext.PolicyContext{}
			policyContext.WithPolicy(&tt.args.policyContext.policy)
			policyContext.WithNewResource(tt.args.policyContext.newResource)

			filteredRuleResp := egn.filterRule(tt.args.rule, logger, policyContext)
			assert.Equal(t, tt.expectedRuleResp, filteredRuleResp)
		})
	}
}

func convertToUnstructured(t *testing.T, obj interface{}) unstructured.Unstructured {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	assert.NoError(t, err)

	return unstructured.Unstructured{
		Object: unstructuredObj,
	}
}
