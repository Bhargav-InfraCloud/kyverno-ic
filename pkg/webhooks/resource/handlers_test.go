package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	kyverno "github.com/kyverno/kyverno/api/kyverno/v1"
	kyvernov1beta1 "github.com/kyverno/kyverno/api/kyverno/v1beta1"
	"github.com/kyverno/kyverno/pkg/config"
	"github.com/kyverno/kyverno/pkg/engine"
	enginecontext "github.com/kyverno/kyverno/pkg/engine/context"
	"github.com/kyverno/kyverno/pkg/engine/jmespath"
	"github.com/kyverno/kyverno/pkg/engine/policycontext"
	log "github.com/kyverno/kyverno/pkg/logging"
	"github.com/kyverno/kyverno/pkg/policycache"
	"github.com/kyverno/kyverno/pkg/webhooks/handlers"
	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
)

var policyCheckLabel = `{
	"apiVersion": "kyverno.io/v1",
	"kind": "ClusterPolicy",
	"metadata": {
	   "name": "check-label-app"
	},
	"spec": {
	   "validationFailureAction": "audit",
	   "rules": [
		  {
			 "name": "check-label-app",
			 "match": {
				"resources": {
				   "kinds": [
					  "Pod"
				   ]
				}
			 },
			 "validate": {
				"message": "The label 'app' is required.",
				"pattern": {
					"metadata": {
						"labels": {
							"app": "?*"
						}
					}
				}
			}
		  }
	   ]
	}
 }
`

var policyInvalid = `{
	"apiVersion": "kyverno.io/v1",
	"kind": "ClusterPolicy",
	"metadata": {
	   "name": "check-label-app"
	},
	"spec": {
	   "validationFailureAction": "audit",
	   "rules": [
		  {
			 "name": "check-label-app",
			 "match": {
				"resources": {
				   "kinds": [
					  "Pod"
				   ]
				}
			 },
			 "validate": {
				"message": "The label 'app' is required.",
				"pattern": {
					"metadata": {
						"labels": {
							"app": "{{ invalid-jmespath }}"
						}
					}
				}
			}
		  }
	   ]
	}
 }
`

var policyVerifySignature = `
{
    "apiVersion": "kyverno.io/v1",
    "kind": "ClusterPolicy",
    "metadata": {
        "name": "check-image",
        "annotations": {
            "pod-policies.kyverno.io/autogen-controllers": "none"
        }
    },
    "spec": {
        "validationFailureAction": "enforce",
        "background": false,
        "webhookTimeoutSeconds": 30,
        "failurePolicy": "Fail",
        "rules": [
            {
                "name": "check-signature",
                "match": {
                    "resources": {
                        "kinds": [
                            "Pod"
                        ]
                    }
                },
                "verifyImages": [
                    {
                        "imageReferences": [
                            "*"
                        ],
                        "attestors": [
                            {
                                "entries": [
                                    {
                                        "keys": {
                                            "publicKeys": "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE8nXRh950IZbRj8Ra/N9sbqOPZrfM\n5/KAQN0/KjHcorm/J5yctVd7iEcnessRQjU917hmKO6JWVGHpDguIyakZA==\n-----END PUBLIC KEY-----"
                                        }
                                    }
                                ]
                            }
                        ]
                    }
                ]
            }
        ]
    }
}
`

var policyMutateAndVerify = `
{
    "apiVersion": "kyverno.io/v1",
    "kind": "ClusterPolicy",
    "metadata": {
        "name": "disallow-unsigned-images"
    },
    "spec": {
        "validationFailureAction": "enforce",
        "background": false,
        "rules": [
            {
                "name": "replace-image-registry",
                "match": {
                    "any": [
                        {
                            "resources": {
                                "kinds": [
                                    "Pod"
                                ]
                            }
                        }
                    ]
                },
                "mutate": {
                    "foreach": [
                        {
                            "list": "request.object.spec.containers",
                            "patchStrategicMerge": {
                                "spec": {
                                    "containers": [
                                        {
                                            "name": "{{ element.name }}",
                                            "image": "{{ regex_replace_all('^([^/]+\\.[^/]+/)?(.*)$', '{{element.image}}', 'ghcr.io/kyverno/$2' )}}"
                                        }
                                    ]
                                }
                            }
                        }
                    ]
                }
            },
            {
                "name": "disallow-unsigned-images-rule",
                "match": {
                    "any": [
                        {
                            "resources": {
                                "kinds": [
                                    "Pod"
                                ]
                            }
                        }
                    ]
                },
                "verifyImages": [
                    {
                        "imageReferences": [
                            "*"
                        ],
                        "verifyDigest": false,
                        "required": null,
                        "mutateDigest": false,
                        "attestors": [
                            {
                                "count": 1,
                                "entries": [
                                    {
                                        "keys": {
                                            "publicKeys": "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE8nXRh950IZbRj8Ra/N9sbqOPZrfM\n5/KAQN0/KjHcorm/J5yctVd7iEcnessRQjU917hmKO6JWVGHpDguIyakZA==\n-----END PUBLIC KEY-----"
                                        }
                                    }
                                ]
                            }
                        ]
                    }
                ]
            }
        ]
    }
}
`

var resourceMutateAndVerify = `{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "labels": {
            "run": "rewrite"
        },
        "name": "rewrite"
    },
    "spec": {
        "containers": [
            {
                "image": "test-verify-image:signed",
                "name": "rewrite",
                "resources": {}
            }
        ],
        "dnsPolicy": "ClusterFirst",
        "restartPolicy": "OnFailure"
    }
}
`

var pod = `{
	"apiVersion": "v1",
	"kind": "Pod",
	"metadata": {
	   "name": "test-pod",
	   "namespace": ""
	},
	"spec": {
	   "containers": [
		  {
			 "name": "nginx",
			 "image": "nginx:latest"
		  }
	   ]
	}
 }
`

var mutateAndGenerateMutatePolicy = `{
  "apiVersion": "kyverno.io/v1",
  "kind": "ClusterPolicy",
  "metadata": {
    "name": "test-mutate"
  },
  "spec": {
    "rules": [
      {
        "name": "test-mutate",
        "match": {
          "any": [
            {
              "resources": {
                "kinds": [
                  "Pod"
                ],
                "operations": [
                  "CREATE"
                ]
              }
            }
          ]
        },
        "mutate": {
          "foreach": [
            {
              "list": "request.object.spec.containers",
              "patchStrategicMerge": {
                "spec": {
                  "containers": [
                    {
                      "name": "{{ element.name }}",
                      "image": "{{ regex_replace_all('^([^/]+\\.[^/]+/)?(.*)$', '{{element.image}}', 'ghcr.io/kyverno/$2' )}}"
                    }
                  ]
                }
              }
            }
          ]
        }
      }
    ]
  }
}`

var mutateAndGenerateGeneratePolicy = `{
  "apiVersion": "kyverno.io/v1",
  "kind": "ClusterPolicy",
  "metadata": {
    "name": "test-generate"
  },
  "spec": {
    "rules": [
      {
        "name": "test-generate",
        "match": {
          "any": [
            {
              "resources": {
                "kinds": [
                  "Pod"
                ],
                "operations": [
                  "CREATE"
                ]
              }
            }
          ]
        },
        "generate": {
          "synchronize": true,
          "apiVersion": "v1",
          "kind": "Pod",
          "name": "pod1-{{request.name}}",
          "namespace": "shared-dp",
          "data": {
            "spec": {
              "containers": [
                {
                  "name": "container",
                  "image": "nginx",
                  "volumeMounts": [
                    {
                      "name": "shared-volume",
                      "mountPath": "/data"
                    }
                  ]
                }
              ]
            }
          }
        }
      }
    ]
  }
}`

var resourceMutateandGenerate = `{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "pod-test-1",
    "namespace": "shared-dp"
  },
  "spec": {
    "containers": [
      {
        "name": "container",
        "image": "nginx"
      }
    ]
  }
}`

func Test_AdmissionResponseValid(t *testing.T) {
	policyCache := policycache.NewCache()
	logger := log.WithName("Test_AdmissionResponseValid")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceHandlers := NewFakeHandlers(ctx, policyCache)

	var validPolicy kyverno.ClusterPolicy
	err := json.Unmarshal([]byte(policyCheckLabel), &validPolicy)
	assert.NilError(t, err)

	key := makeKey(&validPolicy)
	policyCache.Set(key, &validPolicy, policycache.TestResourceFinder{})

	request := handlers.AdmissionRequest{
		AdmissionRequest: v1.AdmissionRequest{
			Operation: v1.Create,
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Object: apiruntime.RawExtension{
				Raw: []byte(pod),
			},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	response := resourceHandlers.Mutate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, true)

	response = resourceHandlers.Validate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, true)
	assert.Equal(t, len(response.Warnings), 0)

	validPolicy.Spec.ValidationFailureAction = "Enforce"
	policyCache.Set(key, &validPolicy, policycache.TestResourceFinder{})

	response = resourceHandlers.Validate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, false)
	assert.Equal(t, len(response.Warnings), 0)

	policyCache.Unset(key)
}

func Test_AdmissionResponseInvalid(t *testing.T) {
	policyCache := policycache.NewCache()
	logger := log.WithName("Test_AdmissionResponseInvalid")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceHandlers := NewFakeHandlers(ctx, policyCache)

	var invalidPolicy kyverno.ClusterPolicy
	err := json.Unmarshal([]byte(policyInvalid), &invalidPolicy)
	assert.NilError(t, err)

	request := handlers.AdmissionRequest{
		AdmissionRequest: v1.AdmissionRequest{
			Operation: v1.Create,
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Object: apiruntime.RawExtension{
				Raw: []byte(pod),
			},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	keyInvalid := makeKey(&invalidPolicy)
	invalidPolicy.Spec.ValidationFailureAction = "Enforce"
	policyCache.Set(keyInvalid, &invalidPolicy, policycache.TestResourceFinder{})

	response := resourceHandlers.Validate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, false)
	assert.Equal(t, len(response.Warnings), 0)

	var ignore kyverno.FailurePolicyType = kyverno.Ignore
	invalidPolicy.Spec.FailurePolicy = &ignore
	policyCache.Set(keyInvalid, &invalidPolicy, policycache.TestResourceFinder{})

	response = resourceHandlers.Validate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, true)
	assert.Equal(t, len(response.Warnings), 1)
}

func Test_ImageVerify(t *testing.T) {
	policyCache := policycache.NewCache()
	logger := log.WithName("Test_ImageVerify")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceHandlers := NewFakeHandlers(ctx, policyCache)

	var policy kyverno.ClusterPolicy
	err := json.Unmarshal([]byte(policyVerifySignature), &policy)
	assert.NilError(t, err)

	key := makeKey(&policy)
	policyCache.Set(key, &policy, policycache.TestResourceFinder{})

	request := handlers.AdmissionRequest{
		AdmissionRequest: v1.AdmissionRequest{
			Operation: v1.Create,
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Object: apiruntime.RawExtension{
				Raw: []byte(pod),
			},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	policy.Spec.ValidationFailureAction = "Enforce"
	policyCache.Set(key, &policy, policycache.TestResourceFinder{})

	response := resourceHandlers.Mutate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, false)
	assert.Equal(t, len(response.Warnings), 0)

	var ignore kyverno.FailurePolicyType = kyverno.Ignore
	policy.Spec.FailurePolicy = &ignore
	policyCache.Set(key, &policy, policycache.TestResourceFinder{})

	response = resourceHandlers.Mutate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, false)
	assert.Equal(t, len(response.Warnings), 0)
}

func Test_MutateAndVerify(t *testing.T) {
	policyCache := policycache.NewCache()
	logger := log.WithName("Test_MutateAndVerify")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceHandlers := NewFakeHandlers(ctx, policyCache)

	var policy kyverno.ClusterPolicy
	err := json.Unmarshal([]byte(policyMutateAndVerify), &policy)
	assert.NilError(t, err)

	key := makeKey(&policy)
	policyCache.Set(key, &policy, policycache.TestResourceFinder{})

	request := handlers.AdmissionRequest{
		AdmissionRequest: v1.AdmissionRequest{
			Operation: v1.Create,
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "Pod"},
			Object: apiruntime.RawExtension{
				Raw: []byte(resourceMutateAndVerify),
			},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	}

	response := resourceHandlers.Mutate(ctx, logger, request, "", time.Now())
	assert.Equal(t, response.Allowed, true)
	assert.Equal(t, len(response.Warnings), 0)
}

func Test_MutateAndGenerate(t *testing.T) {
	policyCache := policycache.NewCache()
	logger := log.WithName("Test_MutateAndGenerate")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resourceHandlers := NewFakeHandlers(ctx, policyCache)

	cfg := config.NewDefaultConfiguration(false)
	jp := jmespath.New(cfg)
	mockPcBuilder := newMockPolicyContextBuilder(cfg, jp)
	resourceHandlers.pcBuilder = mockPcBuilder

	var generatePolicy kyverno.ClusterPolicy
	err := json.Unmarshal([]byte(mutateAndGenerateGeneratePolicy), &generatePolicy)
	assert.NilError(t, err)

	key := makeKey(&generatePolicy)
	policyCache.Set(key, &generatePolicy, policycache.TestResourceFinder{})

	var mutatePolicy kyverno.ClusterPolicy
	err = json.Unmarshal([]byte(mutateAndGenerateMutatePolicy), &mutatePolicy)
	assert.NilError(t, err)

	key = makeKey(&mutatePolicy)
	policyCache.Set(key, &mutatePolicy, policycache.TestResourceFinder{})

	request := handlers.AdmissionRequest{
		AdmissionRequest: v1.AdmissionRequest{
			Operation: v1.Create,
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "Pod"},
			Object: apiruntime.RawExtension{
				Raw: []byte(resourceMutateandGenerate),
			},
			RequestResource: &metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			DryRun:          pointer.Bool(false),
		},
	}

	response := resourceHandlers.Validate(ctx, logger, request, "", time.Now())

	assert.Assert(t, len(mockPcBuilder.contexts) >= 3, fmt.Sprint("expected no of context ", 3, " received ", len(mockPcBuilder.contexts)))

	validateJSONContext := mockPcBuilder.contexts[0].JSONContext()
	mutateJSONContext := mockPcBuilder.contexts[1].JSONContext()
	generateJSONContext := mockPcBuilder.contexts[2].JSONContext()

	_, err = enginecontext.AddMockDeferredLoader(validateJSONContext, "key1", "value1")
	assert.NilError(t, err)
	_, err = enginecontext.AddMockDeferredLoader(mutateJSONContext, "key2", "value2")
	assert.NilError(t, err)
	_, err = enginecontext.AddMockDeferredLoader(generateJSONContext, "key3", "value3")
	assert.NilError(t, err)

	_, err = mutateJSONContext.Query("key1")
	assert.ErrorContains(t, err, `Unknown key "key1" in path`)
	_, err = generateJSONContext.Query("key1")
	assert.ErrorContains(t, err, `Unknown key "key1" in path`)

	_, err = validateJSONContext.Query("key2")
	assert.ErrorContains(t, err, `Unknown key "key2" in path`)
	_, err = generateJSONContext.Query("key2")
	assert.ErrorContains(t, err, `Unknown key "key2" in path`)

	_, err = validateJSONContext.Query("key3")
	assert.ErrorContains(t, err, `Unknown key "key3" in path`)
	_, err = mutateJSONContext.Query("key3")
	assert.ErrorContains(t, err, `Unknown key "key3" in path`)

	assert.Equal(t, response.Allowed, true)
	assert.Equal(t, len(response.Warnings), 0)
}

func makeKey(policy kyverno.PolicyInterface) string {
	name := policy.GetName()
	namespace := policy.GetNamespace()
	if namespace == "" {
		return name
	}

	return namespace + "/" + name
}

type mockPolicyContextBuilder struct {
	configuration config.Configuration
	jp            jmespath.Interface
	contexts      []*engine.PolicyContext
	count         int
}

func newMockPolicyContextBuilder(
	configuration config.Configuration,
	jp jmespath.Interface,
) *mockPolicyContextBuilder {
	return &mockPolicyContextBuilder{
		configuration: configuration,
		jp:            jp,
		contexts:      make([]*policycontext.PolicyContext, 0),
		count:         0,
	}
}

func (b *mockPolicyContextBuilder) Build(request admissionv1.AdmissionRequest, roles, clusterRoles []string, gvk schema.GroupVersionKind) (*engine.PolicyContext, error) {
	userRequestInfo := kyvernov1beta1.RequestInfo{
		AdmissionUserInfo: *request.UserInfo.DeepCopy(),
		Roles:             roles,
		ClusterRoles:      clusterRoles,
	}
	pc, err := engine.NewPolicyContextFromAdmissionRequest(b.jp, request, userRequestInfo, gvk, b.configuration)
	if err != nil {
		return nil, err
	}
	b.count += 1
	b.contexts = append(b.contexts, pc)
	return pc, err
}
