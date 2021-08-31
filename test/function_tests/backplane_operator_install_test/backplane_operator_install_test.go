// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
	"io/ioutil"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"
)

const (
	BackplaneConfigName = "backplane"
	installTimeout      = time.Minute * 5
	duration            = time.Second * 1
	interval            = time.Millisecond * 250
)

var (
	ctx                = context.Background()
	globalsInitialized = false
	baseURL            = ""

	k8sClient client.Client

	backplaneConfig = types.NamespacedName{}

	blockCreationResources = []struct {
		Name     string
		GVK      schema.GroupVersionKind
		Filepath string
		crdPath  string
		Expected string
	}{
		{
			Name: "MultiClusterHub",
			GVK: schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "MultiClusterHub",
			},
			Filepath: "../resources/multiclusterhub.yaml",
			crdPath:  "../resources/multiclusterhub_crd.yaml",
			Expected: "Existing MultiClusterHub resources must first be deleted",
		},
	}
	blockDeletionResources = []struct {
		Name     string
		GVK      schema.GroupVersionKind
		Filepath string
		crdPath  string
		Expected string
	}{
		{
			Name: "BareMetalAsset",
			GVK: schema.GroupVersionKind{
				Group:   "inventory.open-cluster-management.io",
				Version: "v1alpha1",
				Kind:    "BareMetalAsset",
			},
			Filepath: "../resources/baremetalassets.yaml",
			Expected: "Existing BareMetalAsset resources must first be deleted",
		},
		{
			Name: "MultiClusterObservability",
			GVK: schema.GroupVersionKind{
				Group:   "observability.open-cluster-management.io",
				Version: "v1beta2",
				Kind:    "MultiClusterObservability",
			},
			crdPath:  "../resources/multiclusterobservabilities_crd.yaml",
			Filepath: "../resources/multiclusterobservability.yaml",
			Expected: "Existing MultiClusterObservability resources must first be deleted",
		},
		{
			Name: "ManagedCluster",
			GVK: schema.GroupVersionKind{
				Group:   "cluster.open-cluster-management.io",
				Version: "v1",
				Kind:    "ManagedClusterList",
			},
			Filepath: "../resources/managedcluster.yaml",
			Expected: "Existing ManagedCluster resources must first be deleted",
		},
	}
)

func initializeGlobals() {
	// baseURL = *BaseURL
	backplaneConfig = types.NamespacedName{
		Name: BackplaneConfigName,
	}
}

var _ = Describe("BackplaneConfig Test Suite", func() {

	BeforeEach(func() {
		if !globalsInitialized {
			initializeGlobals()
			globalsInitialized = true
		}
	})

	Context("Creating a BackplaneConfig", func() {
		It("Should install all components ", func() {
			By("By creating a new BackplaneConfig", func() {
				Expect(k8sClient.Create(ctx, defaultBackplaneConfig())).Should(Succeed())
			})
		})

		It("Should check that all components were installed correctly", func() {
			By("Ensuring the BackplaneConfig becomes available", func() {
				Eventually(func() bool {
					key := &backplane.BackplaneConfig{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name: BackplaneConfigName,
					}, key)
					return key.Status.Phase == backplane.BackplanePhaseAvailable
				}, installTimeout, interval).Should(BeTrue())

			})
		})

		It("Should check for a healthy status", func() {
			config := &backplane.BackplaneConfig{}
			Expect(k8sClient.Get(ctx, backplaneConfig, config)).To(Succeed())

			By("Checking the phase", func() {
				Expect(config.Status.Phase).To(Equal(backplane.BackplanePhaseAvailable))
			})
			By("Checking the components", func() {
				Expect(len(config.Status.Components)).Should(BeNumerically(">=", 6), "Expected at least 6 components in status")
			})
			By("Checking the conditions", func() {
				available := backplane.BackplaneCondition{}
				for _, c := range config.Status.Conditions {
					if c.Type == backplane.BackplaneAvailable {
						available = c
					}
				}
				Expect(available.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		It("Should ensure validatingwebhook blocks deletion if resouces exist", func() {
			for _, r := range blockDeletionResources {
				By("Creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath)
						defer deleteResource(r.crdPath)
					}
					applyResource(r.Filepath)
					defer deleteResource(r.Filepath)

					config := &backplane.BackplaneConfig{}
					Expect(k8sClient.Get(ctx, backplaneConfig, config)).To(Succeed()) // Get Backplaneconfig

					err := k8sClient.Delete(ctx, config) // Attempt to delete backplaneconfig. Ensure it does not succeed.
					Expect(err).ShouldNot(BeNil())
					Expect(err.Error()).Should(ContainSubstring(r.Expected))
				})
			}
		})

		It("Should ensure validatingwebhook blocks creation if resouces exist", func() {
			for _, r := range blockCreationResources {
				By("Creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath)
						defer deleteResource(r.crdPath)
					}
					applyResource(r.Filepath)
					defer deleteResource(r.Filepath)

					backplaneConfig := defaultBackplaneConfig()
					backplaneConfig.Name = "test"

					err := k8sClient.Create(ctx, backplaneConfig)
					Expect(err).ShouldNot(BeNil())
					Expect(err.Error()).Should(ContainSubstring(r.Expected))
				})
			}
		})
	})
})

func applyResource(resourceFile string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	Expect(k8sClient.Create(ctx, unstructured)).Should(Succeed()) // Create resource on cluster
}

func deleteResource(resourceFile string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	Expect(k8sClient.Delete(ctx, unstructured)).Should(Succeed()) // Delete resource on cluster
}

func defaultBackplaneConfig() *backplane.BackplaneConfig {
	return &backplane.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: BackplaneConfigName,
		},
		Spec: backplane.BackplaneConfigSpec{
			Foo: "bar",
		},
		Status: backplane.BackplaneConfigStatus{
			Phase: "",
		},
	}
}