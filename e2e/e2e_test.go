// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"kpt.dev/resourcegroup/controllers/resourcegroup"
	"kpt.dev/resourcegroup/controllers/root"
	"sigs.k8s.io/cli-utils/pkg/common"
	kstatus "sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "resourcegroup-e2e"
	pollInterval  = 1 * time.Second
	waitTimeout   = 160 * time.Second
)

var _ = Describe("ResourceGroup Controller e2e test", func() {
	It("Test ResourceGroup controller in kpt group", func() {
		By("Creating a resourcegroup")
		rgname := "group-a"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Updating resources")
		resources := []v1alpha1.ObjMetadata{
			{
				Name:      "example",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
		}
		err := patchResources(kubeClient, rgname, resources)
		Expect(err).NotTo(HaveOccurred())

		By("Creating another resourcegroup and adding it as a subgroup")
		subGroupName := "group-b"
		rg := newKptResourceGroup(kubeClient, subGroupName, subGroupName)

		By("Updating subgroup")
		subgroups := []v1alpha1.GroupMetadata{
			{
				Name:      rg.Name,
				Namespace: rg.Namespace,
			},
		}
		err = patchSubgroups(kubeClient, rgname, subgroups)
		Expect(err).NotTo(HaveOccurred())

		By("Updating resourcegroup")
		expectedStatus.ObservedGeneration = 3
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.NotFound,
			},
		}
		expectedStatus.SubgroupStatuses = []v1alpha1.GroupStatus{
			{
				GroupMetadata: v1alpha1.GroupMetadata{
					Namespace: rg.Namespace,
					Name:      rg.Name,
				},
				Status: v1alpha1.Current,
				Conditions: []v1alpha1.Condition{
					{
						Type:    v1alpha1.Ownership,
						Status:  v1alpha1.UnknownConditionStatus,
						Reason:  v1alpha1.OwnershipEmpty,
						Message: "This object is not owned by any inventory object.",
					},
				},
			},
		}

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Creating resources")
		applyResources(kubeClient, resources, rgname)

		expectedStatus.ObservedGeneration = 3
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
		}

		By("The status should contain the resources")
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Update resources with a different owning-inventory")
		applyResources(kubeClient, resources, "another")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
				Conditions: []v1alpha1.Condition{
					{
						Type:    v1alpha1.Ownership,
						Status:  v1alpha1.TrueConditionStatus,
						Reason:  v1alpha1.OwnershipUnmatch,
						Message: "This resource is owned by another ResourceGroup another.",
					},
				},
			},
		}

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname)
		By("Delete subgroup ")
		deleteResourceGroup(kubeClient, subGroupName)

		By("The resource-group-controller pod doesn't get restarted")
		err = controllerPodsNoRestart(clientSet)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ResourceGroup controller stress test with 1000 configmap", func() {
		By("Creating a resourcegroup")
		rgname := "group-c"
		newKptResourceGroup(kubeClient, rgname, "")

		By("Updating resources")
		resources := makeNConfigMaps(1000)
		err := patchResources(kubeClient, rgname, resources)
		Expect(err).NotTo(HaveOccurred())

		By("Updating resourcegroup")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = makeStatusForNConfigMap(1000, v1alpha1.NotFound)

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Creating resources")
		createResources(kubeClient, resources)

		By("The status should contain the resources")
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = makeStatusForNConfigMap(1000, v1alpha1.Current)

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Randomly delete one ConfigMap from 1000 ConfigMaps")
		//nolint:gosec // doesn't need to be cryptographically random
		index := rand.Intn(1000)
		deleteResources(kubeClient, []v1alpha1.ObjMetadata{resources[index-1]})
		expectedStatus.ResourceStatuses[index-1].Status = v1alpha1.NotFound

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Disable status of the resourcegroup")
		err = patchStatusDisabled(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())

		// update the resources list since adding annotation doesn't trigger
		// a reconcile
		resources = resources[1:]
		err = patchResources(kubeClient, rgname, resources)
		Expect(err).NotTo(HaveOccurred())

		expectedStatus = v1alpha1.ResourceGroupStatus{}
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname)

		By("The resource-group-controller pod doesn't get restarted")
		err = controllerPodsNoRestart(clientSet)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Test ResourceGroup controller with CRD v1beta1", func() {
		// Lookup CRD versions supported by the server
		mappings, err := mapper.RESTMappings(schema.GroupKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"})
		Expect(err).NotTo(HaveOccurred())
		versions := make(map[string]struct{}, len(mappings))
		for _, mapping := range mappings {
			versions[mapping.GroupVersionKind.Version] = struct{}{}
		}
		if _, ok := versions["v1beta1"]; !ok {
			Skip("Server does not support apiextensions.k8s.io/v1beta1")
		}

		By("Apply the v1beta1 CRD for ResourceGroup")
		err = runMake("apply-v1beta1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1beta1 ResourceGroup CRD: %v", err)

		By("Wait for the CRD to be ready")
		waitForCRD(kubeClient, "resourcegroups.kpt.dev")

		// Reset RESTMapper to pick up the new CRD version.
		mapper.Reset()

		By("Apply a ResourceGroup CR")
		rgname := "crd-version-1"
		rg := newKptResourceGroup(kubeClient, rgname, rgname)

		By("We can get the applied ResourceGroup CR")
		clusterRG := rg.DeepCopy()
		err = kubeClient.Get(context.TODO(), client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, clusterRG)
		Expect(err).NotTo(HaveOccurred(), "failed to get RG")
	})

	It("Test ResourceGroup controller with CRD v1", func() {
		// Lookup CRD versions supported by the server
		mappings, err := mapper.RESTMappings(schema.GroupKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"})
		Expect(err).NotTo(HaveOccurred())
		versions := make(map[string]struct{}, len(mappings))
		for _, mapping := range mappings {
			versions[mapping.GroupVersionKind.Version] = struct{}{}
		}
		if _, ok := versions["v1"]; !ok {
			Fail("Server does not support apiextensions.k8s.io/v1")
		}

		By("Apply the v1 CRD for ResourceGroup")
		err = runMake("apply-v1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1 ResourceGroup CRD: %v", err)

		By("Wait for the v1 CRD to be ready")
		waitForCRD(kubeClient, "resourcegroups.kpt.dev")

		// Reset RESTMapper to pick up the new CRD version.
		mapper.Reset()

		By("Apply a ResourceGroup CR")
		rgname := "crd-version-2"
		rg := newKptResourceGroup(kubeClient, rgname, rgname)

		By("We can get the applied ResourceGroup CR")
		clusterRG := rg.DeepCopy()
		err = kubeClient.Get(context.TODO(), client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, clusterRG)
		Expect(err).NotTo(HaveOccurred(), "failed to get RG")
	})

	It("Test ResourceGroup upgrade from CRD v1beta1 to v1", func() {
		// Lookup CRD versions supported by the server
		mappings, err := mapper.RESTMappings(schema.GroupKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"})
		Expect(err).NotTo(HaveOccurred())
		versions := make(map[string]struct{}, len(mappings))
		for _, mapping := range mappings {
			versions[mapping.GroupVersionKind.Version] = struct{}{}
		}
		if _, ok := versions["v1"]; !ok {
			Fail("Server does not support apiextensions.k8s.io/v1")
		}
		if _, ok := versions["v1beta1"]; !ok {
			Skip("Server does not support apiextensions.k8s.io/v1beta1")
		}

		By("Apply the v1beta1 CRD for ResourceGroup")
		err = runMake("apply-v1beta1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1beta1 ResourceGroup CRD: %v", err)

		By("Wait for the CRD to be ready")
		waitForCRD(kubeClient, "resourcegroups.kpt.dev")

		// Reset RESTMapper to pick up the new CRD version.
		mapper.Reset()

		By("Apply a ResourceGroup CR")
		rgname := "crd-version-3"
		rg := newKptResourceGroup(kubeClient, rgname, rgname)

		By("Apply the v1 CRD for ResourceGroup")
		err = runMake("apply-v1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1 ResourceGroup CRD: %v", err)

		By("Wait for the v1 CRD to be ready")
		waitForCRD(kubeClient, "resourcegroups.kpt.dev")

		// Reset RESTMapper to pick up the new CRD version.
		mapper.Reset()

		By("We can get the applied ResourceGroup CR")
		clusterRG := rg.DeepCopy()
		err = kubeClient.Get(context.TODO(), client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, clusterRG)
		Expect(err).NotTo(HaveOccurred(), "failed to get RG after upgrading CRD version")
	})

	It("Test ResourceGroup controller for KCC resources", func() {
		// Lookup KCC CRD versions supported by the server
		mappings, err := mapper.RESTMappings(schema.GroupKind{Group: "serviceusage.cnrm.cloud.google.com", Kind: "Service"})
		if err != nil && meta.IsNoMatchError(err) {
			Skip("Server does not have Config Connector installed")
		}
		Expect(err).NotTo(HaveOccurred())
		versions := make(map[string]struct{}, len(mappings))
		for _, mapping := range mappings {
			versions[mapping.GroupVersionKind.Version] = struct{}{}
		}
		if _, ok := versions["v1beta1"]; !ok {
			Skip("Server does not have Config Connector installed")
		}

		By("Creating a resourcegroup")
		rgname := "group-kcc"
		newKptResourceGroup(kubeClient, rgname, "")

		By("Adding a KCC resource")
		resource := createKCCResource(kubeClient)

		defer deleteKCCResource(kubeClient)

		By("Updating resourcegroup")
		err = patchResources(kubeClient, rgname, []v1alpha1.ObjMetadata{resource})
		Expect(err).NotTo(HaveOccurred())

		By("Verifying the resourcegroup status: It can surface the kcc resource status.conditions")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			makeKCCResourceStatus(resource),
		}

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Delete the resourcegroup")
		deleteResourceGroup(kubeClient, rgname)
	})

	It("Test CustomResource", func() {
		By("Creating a resourcegroup")
		rgname := "group-d"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Updating resources")
		resources := []v1alpha1.ObjMetadata{
			{
				Name:      "example",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "test.io",
					Kind:  "TestCase",
				},
			},
		}
		err := patchResources(kubeClient, rgname, resources)
		Expect(err).NotTo(HaveOccurred())

		By("Validating resourcegroup")
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.NotFound,
			},
		}

		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Creating CRD")
		applyCRD(kubeClient)
		By("Creating CR")
		applyCR()

		By("Validating resource status updated")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.Current,
				Conditions: []v1alpha1.Condition{
					{
						Type:    v1alpha1.Ownership,
						Status:  v1alpha1.UnknownConditionStatus,
						Reason:  "Unknown",
						Message: "This object is not owned by any inventory object.",
					},
				},
			},
		}
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Deleting CRD")
		deleteCRD(kubeClient)
		By("Validating resource status updated")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.NotFound,
			},
		}
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)
	})

	It("Test ResourceGroup controller consumes apply status for resources", func() {
		By("Creating a resourcegroup")
		rgname := "group-e"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("Creating and applying CM resources")
		resources := []v1alpha1.ObjMetadata{
			{
				Name:      "test-status-0",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-1",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-2",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-3",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-4",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-5",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
			{
				Name:      "test-status-6",
				Namespace: testNamespace,
				GroupKind: v1alpha1.GroupKind{
					Group: "",
					Kind:  "ConfigMap",
				},
			},
		}
		// Apply all but the last CM resource to test NotFound error.
		applyResources(kubeClient, resources[:len(resources)-1], rgname)

		err := patchResources(kubeClient, rgname, resources)
		Expect(err).NotTo(HaveOccurred())

		By("The status field should use the old logic when actuation/strategy status is not set")
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[1],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[2],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[3],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[4],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[5],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[6],
				Status:      v1alpha1.NotFound,
			},
		}
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		By("The status should contain the resources with correct status set by controller when actuation and strategy is set/injected")
		injectStatuses := []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[1],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationPending,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[2],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationFailed,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[3],
				Strategy:    v1alpha1.Delete,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[4],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[5],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.Unknown,
			},
			{
				// CM should not exist on the live cluster.
				ObjMetadata: resources[6],
			},
		}

		resourceVersionBefore, err := getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())

		err = patchResourceStatuses(kubeClient, rgname, injectStatuses)
		Expect(err).NotTo(HaveOccurred())

		// ResourceVersion should change with a spec update
		resourceVersionAfter, err := getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceVersionAfter).NotTo(BeIdenticalTo(resourceVersionBefore))
		resourceVersionBefore = resourceVersionAfter

		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[1],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationPending,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.Unknown,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[2],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationFailed,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.Unknown,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[3],
				Strategy:    v1alpha1.Delete,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[4],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcilePending,
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[5],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationSucceeded,
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[6],
				Status:      v1alpha1.NotFound,
			},
		}
		waitForResourceGroupStatus(kubeClient, expectedStatus, rgname)

		// ResourceVersion should change with a status update
		resourceVersionAfter, err = getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceVersionAfter).NotTo(BeIdenticalTo(resourceVersionBefore))
		resourceVersionBefore = resourceVersionAfter

		// Wait and check to see we don't cause an infinite/recursive reconcile loop
		// by ensuring the resourceVersion doesn't change.
		time.Sleep(60 * time.Second)
		resourceVersionAfter, err = getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceVersionAfter).To(BeIdenticalTo(resourceVersionBefore))

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname)

		By("The resource-group-controller pod doesn't get restarted")
		err = controllerPodsNoRestart(clientSet)
		Expect(err).NotTo(HaveOccurred())
	})

})

func newKptResourceGroup(kubeClient client.Client, name, id string) *v1alpha1.ResourceGroup {
	rg := &v1alpha1.ResourceGroup{}
	rg.SetNamespace(testNamespace)
	rg.SetName(name)
	if id != "" {
		rg.SetLabels(map[string]string{
			common.InventoryLabel: id,
		})
	}
	By("Create ResourceGroup " + name)
	err := kubeClient.Create(context.TODO(), rg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create ResourceGroup %s", name)
	return rg
}

func deleteResourceGroup(kubeClient client.Client, name string) {
	rg := &v1alpha1.ResourceGroup{}
	rg.SetNamespace(testNamespace)
	rg.SetName(name)

	By("Delete ResourceGroup " + name)
	err := kubeClient.Delete(context.TODO(), rg)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete ResourceGroup %s", name)
}

func createResources(kubeClient client.Client, resources []v1alpha1.ObjMetadata) {
	for _, r := range resources {
		u := &unstructured.Unstructured{}
		u.SetName(r.Name)
		u.SetNamespace(r.Namespace)
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   r.Group,
			Version: "v1",
			Kind:    r.Kind,
		})
		err := kubeClient.Create(context.TODO(), u)
		Expect(err).NotTo(HaveOccurred(), "Failed to create resource %s: %v", r.Name, err)
	}
}

func applyCRD(kubeClient client.Client) {
	err := applyTestFile("testdata/testcase_crd.yaml")
	Expect(err).ShouldNot(HaveOccurred())
	waitForCRD(kubeClient, "testcases.test.io")
}

func deleteCRD(kubeClient client.Client) {
	resources := []v1alpha1.ObjMetadata{
		{
			Name: "testcases.test.io",
			GroupKind: v1alpha1.GroupKind{
				Group: "apiextensions.k8s.io",
				Kind:  "CustomResourceDefinition",
			},
		},
	}
	deleteResources(kubeClient, resources)
}

func applyCR() {
	err := applyTestFile("testdata/testcase_cr.yaml")
	Expect(err).ShouldNot(HaveOccurred())
}

func applyResources(kubeClient client.Client, resources []v1alpha1.ObjMetadata, id string) {
	for _, r := range resources {
		u := &unstructured.Unstructured{}
		u.SetName(r.Name)
		u.SetNamespace(r.Namespace)
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   r.Group,
			Version: "v1",
			Kind:    r.Kind,
		})
		u.SetAnnotations(map[string]string{
			"config.k8s.io/owning-inventory":      id,
			resourcegroup.SourceHashAnnotationKey: "1234567890",
		})

		err := kubeClient.Get(context.TODO(), client.ObjectKey{Name: r.Name, Namespace: r.Namespace}, u.DeepCopy())
		if err != nil {
			Expect(kubeerrors.IsNotFound(err)).Should(Equal(true))
			err = kubeClient.Create(context.TODO(), u)
			Expect(err).NotTo(HaveOccurred(), "Failed to create resource %s: %v", r.Name, err)
		} else {
			err = kubeClient.Update(context.TODO(), u)
			Expect(err).NotTo(HaveOccurred(), "Failed to update resource %s: %v", r.Name, err)
		}
	}
}

func makeNConfigMaps(n int) []v1alpha1.ObjMetadata {
	resources := []v1alpha1.ObjMetadata{}
	for i := 1; i <= n; i++ {
		r := v1alpha1.ObjMetadata{
			Name:      fmt.Sprintf("example%d", i),
			Namespace: testNamespace,
			GroupKind: v1alpha1.GroupKind{
				Group: "",
				Kind:  "ConfigMap",
			},
		}
		resources = append(resources, r)
	}
	return resources
}

func deleteResources(kubeClient client.Client, resources []v1alpha1.ObjMetadata) {
	for _, r := range resources {
		u := &unstructured.Unstructured{}
		u.SetName(r.Name)
		u.SetNamespace(r.Namespace)
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   r.Group,
			Version: "v1",
			Kind:    r.Kind,
		})
		err := kubeClient.Delete(context.TODO(), u)
		Expect(err).NotTo(HaveOccurred(), "Failed to delete resource %s: %v", r.Name, err)
	}
}

func initResourceStatus() v1alpha1.ResourceGroupStatus {
	return v1alpha1.ResourceGroupStatus{
		Conditions: []v1alpha1.Condition{
			{
				Type:    v1alpha1.Reconciling,
				Status:  v1alpha1.FalseConditionStatus,
				Reason:  resourcegroup.FinishReconciling,
				Message: "finish reconciling",
			},
			{
				Type:    v1alpha1.Stalled,
				Status:  v1alpha1.FalseConditionStatus,
				Reason:  resourcegroup.FinishReconciling,
				Message: "finish reconciling",
			},
		},
	}
}

func makeStatusForNConfigMap(n int, s v1alpha1.Status) []v1alpha1.ResourceStatus {
	resourceStatus := []v1alpha1.ResourceStatus{}
	for i := 1; i <= n; i++ {
		r := v1alpha1.ObjMetadata{
			Name:      fmt.Sprintf("example%d", i),
			Namespace: testNamespace,
			GroupKind: v1alpha1.GroupKind{
				Group: "",
				Kind:  "ConfigMap",
			},
		}
		status := v1alpha1.ResourceStatus{
			ObjMetadata: r,
			Status:      s,
		}
		resourceStatus = append(resourceStatus, status)
	}
	return resourceStatus
}

//nolint:unparam
func getResourceVersion(kubeClient client.Client, name string) (string, error) {
	rg := &v1alpha1.ResourceGroup{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
	if err != nil {
		if kubeerrors.IsNotFound(err) {
			return "", nil
		}

		return "", err
	}

	return rg.ResourceVersion, nil
}

func waitForResourceGroupStatus(kubeClient client.Client, status v1alpha1.ResourceGroupStatus, name string) {
	EventuallyWithOffset(1, func() v1alpha1.ResourceGroupStatus {
		obj := &v1alpha1.ResourceGroup{}
		obj.SetNamespace(testNamespace)
		obj.SetName(name)

		err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		if err != nil && kubeerrors.IsNotFound(err) {
			return v1alpha1.ResourceGroupStatus{}
		}
		Expect(err).ToNot(HaveOccurred())

		return obj.Status
	}, pollInterval).WithTimeout(waitTimeout).Should(Asserter.EqualMatcher(status))
}

func patchResourceGroup(kubeClient client.Client, name string, modify func(*v1alpha1.ResourceGroup)) error {
	obj := &v1alpha1.ResourceGroup{}
	obj.SetNamespace(testNamespace)
	obj.SetName(name)

	err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return err
	}

	obj2 := obj.DeepCopy()
	modify(obj2)

	err = kubeClient.Patch(context.TODO(), obj2, client.MergeFrom(obj))
	if err != nil {
		return err
	}
	return nil
}

func patchResourceGroupStatus(kubeClient client.Client, name string, modify func(*v1alpha1.ResourceGroup)) error {
	obj := &v1alpha1.ResourceGroup{}
	obj.SetNamespace(testNamespace)
	obj.SetName(name)

	err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return err
	}

	obj2 := obj.DeepCopy()
	modify(obj2)

	err = kubeClient.Status().Patch(context.TODO(), obj2, client.MergeFrom(obj))
	if err != nil {
		return err
	}
	return nil
}

func patchResourceStatuses(kubeClient client.Client, name string, statuses []v1alpha1.ResourceStatus) error {
	return patchResourceGroupStatus(kubeClient, name, func(obj *v1alpha1.ResourceGroup) {
		obj.Status.ResourceStatuses = statuses
	})
}

func patchResources(kubeClient client.Client, name string, resources []v1alpha1.ObjMetadata) error {
	return patchResourceGroup(kubeClient, name, func(obj *v1alpha1.ResourceGroup) {
		obj.Spec.Resources = resources
	})
}

func patchStatusDisabled(kubeClient client.Client, name string) error {
	return patchResourceGroup(kubeClient, name, func(obj *v1alpha1.ResourceGroup) {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		annotations[root.DisableStatusKey] = root.DisableStatusValue
		obj.SetAnnotations(annotations)
	})
}

func patchSubgroups(kubeClient client.Client, name string, subgroups []v1alpha1.GroupMetadata) error {
	obj := &v1alpha1.ResourceGroup{}
	obj.SetNamespace(testNamespace)
	obj.SetName(name)

	err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
	if err != nil {
		return err
	}

	obj2 := obj.DeepCopy()
	obj2.Spec.Subgroups = subgroups

	err = kubeClient.Patch(context.TODO(), obj2, client.MergeFrom(obj))
	if err != nil {
		return err
	}
	return nil
}

func waitForObjectStatus(kubeClient client.Client, obj *unstructured.Unstructured, desiredStatus kstatus.Status) {
	EventuallyWithOffset(1, func() kstatus.Status {
		err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		if err != nil && kubeerrors.IsNotFound(err) {
			return kstatus.NotFoundStatus
		}
		Expect(err).ToNot(HaveOccurred())

		result, err := kstatus.Compute(obj)
		Expect(err).ToNot(HaveOccurred())

		return result.Status
	}, pollInterval).WithTimeout(waitTimeout).Should(Equal(desiredStatus))
}

func waitForDeploymentCurrent(kubeClient client.Client, namespace, name string) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("Deployment"))
	obj.SetNamespace(namespace)
	obj.SetName(name)
	waitForObjectStatus(kubeClient, obj, kstatus.CurrentStatus)
}

func waitForNamespaceCurrent(kubeClient client.Client, namespace string) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	obj.SetName(namespace)
	waitForObjectStatus(kubeClient, obj, kstatus.CurrentStatus)
}

func waitForNamespaceNotFound(kubeClient client.Client, namespace string) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	obj.SetName(namespace)
	waitForObjectStatus(kubeClient, obj, kstatus.NotFoundStatus)
}

func deleteNamespace(kubeClient client.Client, namespace string) {
	obj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err := kubeClient.Delete(context.TODO(), obj,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		Expect(kubeerrors.IsNotFound(err)).Should(Equal(true))
	} else {
		Expect(err).NotTo(HaveOccurred())

		By("Wait for namespace to be deleted")
		waitForNamespaceNotFound(kubeClient, namespace)
	}
}

func applyNamespace(kubeClient client.Client, namespace string) {
	obj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err := kubeClient.Create(context.TODO(), obj)
	if err != nil {
		Expect(kubeerrors.IsAlreadyExists(err)).Should(Equal(true))
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	By("Wait for namespace to be ready")
	waitForNamespaceCurrent(kubeClient, namespace)
}

func controllerPodsNoRestart(clientSet *kubernetes.Clientset) error {
	c := clientSet.CoreV1().Pods("resource-group-system")
	pods, err := c.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Status.ContainerStatuses {
			if c.RestartCount != 0 {
				return fmt.Errorf("%s has a restart as %d", c.Name, c.RestartCount)
			}
		}
	}
	return nil
}

func runMake(target string) error {
	//nolint:gosec // all usages use hard-coded strings as input
	cmd := exec.Command("bash", "-c", fmt.Sprintf("make %s", target))
	cmd.Env = os.Environ()
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd.Dir = filepath.Join(wd, "../")
	return runCmd(cmd)
}

func GetLogs() ([]byte, error) {
	cmd := exec.Command("bash", "-c", "kubectl logs Deployment/resource-group-controller-manager -c manager -n resource-group-system")
	cmd.Env = os.Environ()
	return cmd.Output()
}

func applyTestFile(file string) error {
	wd, err := os.Getwd()
	Expect(err).ShouldNot(HaveOccurred())
	//nolint:gosec // all usages use hard-coded strings as input
	cmd := exec.Command("bash", "-c", fmt.Sprintf("kubectl apply -f \"%s\"", file))
	cmd.Env = os.Environ()
	cmd.Dir = wd
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Got an error %s\n", string(output))
	}
	return err
}

func runCmd(cmd *exec.Cmd) error {
	bytes, err := cmd.Output()
	if err != nil {
		fmt.Printf("Stdout: %s \n", string(bytes))

		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Stderr: %s \n", string(exitErr.Stderr))
		}
	}
	return err
}

type byTimestamp []corev1.Event

func (b byTimestamp) Len() int           { return len(b) }
func (b byTimestamp) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byTimestamp) Less(i, j int) bool { return b[i].FirstTimestamp.Before(&b[j].FirstTimestamp) }

func dumpEvents(clientSet *kubernetes.Clientset, namespace string) {
	eventList, err := clientSet.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	fmt.Printf("Found %d events\n", len(eventList.Items))
	sortedEvents := eventList.Items
	sort.Sort(byTimestamp(sortedEvents))

	for _, e := range sortedEvents {
		fmt.Printf("At %v - event for %v: %v %v: %v\n", e.FirstTimestamp, e.InvolvedObject.Name, e.Source, e.Reason, e.Message)
	}
	fmt.Println()
}

func waitForCRD(kubeClient client.Client, name string) {
	EventuallyWithOffset(1, func() kstatus.Status {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
		obj.SetName(name)

		err := kubeClient.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		if err != nil && kubeerrors.IsNotFound(err) {
			return kstatus.NotFoundStatus
		}
		Expect(err).ToNot(HaveOccurred())

		result, err := kstatus.Compute(obj)
		Expect(err).ToNot(HaveOccurred())

		return result.Status
	}, pollInterval).WithTimeout(waitTimeout).Should(Equal(kstatus.CurrentStatus))
}
