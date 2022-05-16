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
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"kpt.dev/resourcegroup/controllers/resourcegroup"
	"kpt.dev/resourcegroup/controllers/root"
)

const (
	testNamespace = "resourcegroup-e2e"
	pollInterval  = 10 * time.Second
	waitTimeout   = 160 * time.Second
)

var (
	config    *rest.Config
	clientSet *kubernetes.Clientset
)

var _ = Describe("ResourceGroup Controller e2e test", func() {
	It("Test ResourceGroup controller in kpt group", func() {
		By("Creating a resourcegroup")
		rgname := "group-a"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		err := waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

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
		_ = updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, resources)

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
		updateResourceGroupSubGroup(kubeClient, rgname, root.KptGroup, subgroups)

		By("Updating resourcegroup")
		expectedStatus.ObservedGeneration = 3
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "NotFound",
			},
		}
		expectedStatus.SubgroupStatuses = []v1alpha1.GroupStatus{
			{
				GroupMetadata: v1alpha1.GroupMetadata{
					Namespace: rg.Namespace,
					Name:      rg.Name,
				},
				Status: "Current",
			},
		}

		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Creating resources")
		applyResources(kubeClient, resources, rgname)

		expectedStatus.ObservedGeneration = 3
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "Current",
				SourceHash:  "1234567",
			},
		}

		By("The status should contain the resources")
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Update resources with a different owning-inventory")
		applyResources(kubeClient, resources, "another")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "Current",
				SourceHash:  "1234567",
				Conditions: []v1alpha1.Condition{
					{
						Message: "This resource is owned by another ResourceGroup another. The status only reflects the specification for the current object in ResourceGroup another.",
						Type:    v1alpha1.Ownership,
						Status:  "True",
					},
				},
			},
		}

		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname, root.KptGroup)
		By("Delete subgroup ")
		deleteResourceGroup(kubeClient, subGroupName, root.KptGroup)

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
		updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, resources)

		By("Updating resourcegroup")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = makeStatusForNConfigMap(1000, "NotFound")

		err := waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Creating resources")
		createResources(kubeClient, resources)

		By("The status should contain the resources")
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = makeStatusForNConfigMap(1000, "Current")

		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Randomly delete one ConfigMap from 1000 ConfigMaps")
		index := rand.Intn(1000)
		deleteResources(kubeClient, []v1alpha1.ObjMetadata{resources[index-1]})
		expectedStatus.ResourceStatuses[index-1].Status = "NotFound"

		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Disable status of the resourcegroup")
		addStatusDisabledAnnotation(kubeClient, rgname)
		// update the resources list since adding annotation doesn't trigger
		// a reconcile
		resources = resources[1:]
		updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, resources)
		expectedStatus = v1alpha1.ResourceGroupStatus{}
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname, root.KptGroup)

		By("The resource-group-controller pod doesn't get restarted")
		err = controllerPodsNoRestart(clientSet)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Test ResourceGroup controller in kpt group", func() {
		By("Apply the v1beta1 CRD for ResourceGroup ")
		err := RunMake("apply-v1beta1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1beta1 ResourceGroup CRD: %v", err)

		config, err = clientConfig()
		Expect(err).NotTo(HaveOccurred())
		kubeClient, err = newKubeClient(config)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for the CRD to be ready")
		err = waitForCRD(kubeClient, "resourcegroups.kpt.dev")
		Expect(err).NotTo(HaveOccurred(), "v1beta1 CRD is not ready")

		By("Apply a ResourceGroup CR")
		rgname := "crd-version"
		rg := newKptResourceGroup(kubeClient, rgname, rgname)

		By("Apply the v1 CRD for ResourceGroup")
		err = RunMake("apply-v1-crd")
		Expect(err).NotTo(HaveOccurred(), "failed to apply v1 ResourceGroup CRD: %v", err)

		By("Wait for the v1 CRD to be ready")
		err = waitForCRD(kubeClient, "resourcegroups.kpt.dev")
		Expect(err).NotTo(HaveOccurred(), "v1 CRD is not ready")

		By("We can get the applied ResourceGroup CR")
		clusterRG := rg.DeepCopy()
		err = kubeClient.Get(context.TODO(), client.ObjectKey{Name: rg.Name, Namespace: rg.Namespace}, clusterRG)
		Expect(err).NotTo(HaveOccurred(), "failed to get RG after upgrading CRD version")
	})

	It("Test ResourceGroup controller for KCC resources", func() {
		By("Creating a resourcegroup")
		rgname := "group-kcc"
		newKptResourceGroup(kubeClient, rgname, "")

		By("Adding a KCC resource")
		resource := createKCCResource(kubeClient)

		defer deleteKCCResource(kubeClient)

		By("Updating resourcegroup")
		updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, []v1alpha1.ObjMetadata{resource})

		By("Verifying the resourcegroup status: It can surface the kcc resource status.conditions")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			makeKCCResourceStatus(resource),
		}

		err := waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Delete the resourcegroup")
		deleteResourceGroup(kubeClient, rgname, root.KptGroup)
	})

	It("Test CustomResource", func() {
		By("Creating a resourcegroup")
		rgname := "group-d"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		err := waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

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
		_ = updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, resources)

		By("Validating resourcegroup")
		expectedStatus.ObservedGeneration = 2
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "NotFound",
			},
		}

		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Creating CRD")
		applyCRD(kubeClient)
		By("Creating CR")
		applyCR()

		By("Validating resource status updated")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "Current",
				Conditions: []v1alpha1.Condition{
					{
						Message: "This object is not owned by any inventory object. The status for the current object may not reflect the specification for it in current ResourceGroup.",
						Type:    v1alpha1.Ownership,
						Status:  "Unknown",
						Reason:  "Unknown",
					},
				},
			},
		}
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting CRD")
		deleteCRD(kubeClient)
		By("Validating resource status updated")
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      "NotFound",
			},
		}
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Test ResourceGroup controller consumes apply status for resources", func() {
		By("Creating a resourcegroup")
		rgname := "group-e"
		newKptResourceGroup(kubeClient, rgname, rgname)

		By("The status of the resourcegroup should be empty")
		expectedStatus := initResourceStatus()
		expectedStatus.ObservedGeneration = 1
		err := waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

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
		_ = updateResourceGroupSpec(kubeClient, rgname, root.KptGroup, resources)

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
				SourceHash:  "1234567",
			},
		}
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())

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
				Reconcile:   v1alpha1.ReconcileSucceeded,
				Status:      v1alpha1.InProgress,
			},
			{
				ObjMetadata: resources[2],
				Strategy:    v1alpha1.Apply,
				Actuation:   v1alpha1.ActuationFailed,
				Reconcile:   v1alpha1.ReconcileSucceeded,
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
		updateResourceGroupResourceStatus(kubeClient, rgname, injectStatuses)
		expectedStatus.ResourceStatuses = []v1alpha1.ResourceStatus{
			{
				ObjMetadata: resources[0],
				Status:      v1alpha1.Current,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[1],
				Status:      v1alpha1.Unknown,
				SourceHash:  "1234567",
			},
			{
				ObjMetadata: resources[2],
				Status:      v1alpha1.Unknown,
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
				SourceHash:  "1234567",
			},
		}
		err = waitResourceGroupStatus(kubeClient, expectedStatus, rgname)
		Expect(err).NotTo(HaveOccurred())
		resourceVersionAfter, err := getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceVersionAfter).NotTo(BeIdenticalTo(resourceVersionBefore))

		// Wait and check to see we don't cause an infinite/recursive reconcile loop
		// by ensuring the resourceVersion doesn't change.
		time.Sleep(60 * time.Second)
		resourceVersionAfterWait, err := getResourceVersion(kubeClient, rgname)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceVersionAfter).To(BeIdenticalTo(resourceVersionAfterWait))

		By("Delete resource group")
		deleteResourceGroup(kubeClient, rgname, root.KptGroup)

		By("The resource-group-controller pod doesn't get restarted")
		err = controllerPodsNoRestart(clientSet)
		Expect(err).NotTo(HaveOccurred())
	})

})

func newKptResourceGroup(kubeClient client.Client, name, id string) v1alpha1.ResourceGroup {
	rg := v1alpha1.ResourceGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      name,
		},
	}
	if id != "" {
		rg.SetLabels(map[string]string{
			common.InventoryLabel: id,
		})
	}
	By("Create ResourceGroup " + name)
	err := kubeClient.Create(context.TODO(), &rg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create ResourceGroup %s", name)
	return rg
}

func deleteResourceGroup(kubeClient client.Client, name, group string) {
	var rg client.Object
	if group == root.ConfigSyncGroup {
		rg = &v1alpha1.ResourceGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      name,
			},
		}
	} else {
		rg = &v1alpha1.ResourceGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      name,
			},
		}
	}

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
	err = waitForCRD(kubeClient, "testcases.test.io")
	Expect(err).ShouldNot(HaveOccurred())
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

func updateResourceGroupResourceStatus(kubeClient client.Client,
	name string, statuses []v1alpha1.ResourceStatus) error {

	rg := &v1alpha1.ResourceGroup{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
	if err != nil {
		if kubeerrors.IsNotFound(err) {
			return fmt.Errorf("resourcegroup %q in namespace %q is not found", name, testNamespace)
		}
		return err
	}

	rg.Status.ResourceStatuses = statuses
	return kubeClient.Status().Update(context.TODO(), rg)
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
				Type:   v1alpha1.Reconciling,
				Status: v1alpha1.FalseConditionStatus,
			},
			{
				Type:   v1alpha1.Stalled,
				Status: v1alpha1.FalseConditionStatus,
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

func waitResourceGroupStatus(kubeClient client.Client, status v1alpha1.ResourceGroupStatus, name string) error {
	return wait.PollImmediate(pollInterval, waitTimeout, func() (bool, error) {
		rg := &v1alpha1.ResourceGroup{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
		if err != nil && kubeerrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to get resource group %v", err)
		}
		liveStatus := rg.Status

		if liveStatus.ObservedGeneration != status.ObservedGeneration {
			return false, nil
		}
		if len(liveStatus.Conditions) != len(status.Conditions) {
			return false, nil
		}
		for i, c := range liveStatus.Conditions {
			expectedC := status.Conditions[i]
			if c.Type == expectedC.Type && c.Status == expectedC.Status {
				continue
			}
			return false, nil
		}
		if len(liveStatus.ResourceStatuses) != len(status.ResourceStatuses) {
			return false, nil
		}
		for i, r := range liveStatus.ResourceStatuses {
			expectedR := status.ResourceStatuses[i]
			if r.Status != expectedR.Status {
				return false, nil
			}
			if len(r.Conditions) != len(expectedR.Conditions) {
				return false, nil
			}
			for i, c := range r.Conditions {
				ec := expectedR.Conditions[i]
				if c.Type != ec.Type {
					return false, nil
				}
				if c.Message != ec.Message && !strings.Contains(c.Message, ec.Message) {
					return false, nil
				}
			}
		}
		if len(liveStatus.SubgroupStatuses) != len(status.SubgroupStatuses) {
			return false, nil
		}
		for i, r := range liveStatus.SubgroupStatuses {
			expectedR := status.SubgroupStatuses[i]
			if r.Status != expectedR.Status {
				return false, nil
			}
		}
		return true, nil
	})
}

func updateResourceGroupSpec(kubeClient client.Client,
	name, group string,
	resources []v1alpha1.ObjMetadata) error {
	var rg client.Object
	rg = &v1alpha1.ResourceGroup{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
	if err != nil && kubeerrors.IsNotFound(err) {
		return fmt.Errorf("resourcegroup is not found")
	}
	if err != nil {
		return err
	}
	if group == root.ConfigSyncGroup {
		r := rg.(*v1alpha1.ResourceGroup)
		r.Spec.Resources = resources
		return kubeClient.Update(context.TODO(), rg)
	} else {
		r := rg.(*v1alpha1.ResourceGroup)
		r.Spec.Resources = resources
		return kubeClient.Update(context.TODO(), rg)
	}
}

func addStatusDisabledAnnotation(kubeClient client.Client, name string) error {
	rg := &v1alpha1.ResourceGroup{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
	if err != nil {
		return err
	}
	annotations := rg.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[root.DisableStatusKey] = root.DisableStatusValue
	rg.SetAnnotations(annotations)
	return kubeClient.Update(context.TODO(), rg)
}

func updateResourceGroupSubGroup(kubeClient client.Client,
	name, group string,
	subgroups []v1alpha1.GroupMetadata) error {
	var rg client.Object
	if group == root.ConfigSyncGroup {
		rg = &v1alpha1.ResourceGroup{}
	} else {
		rg = &v1alpha1.ResourceGroup{}
	}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, rg)
	if err != nil && kubeerrors.IsNotFound(err) {
		return fmt.Errorf("resourcegroup is not found")
	}
	if err != nil {
		return err
	}
	if group == root.ConfigSyncGroup {
		r := rg.(*v1alpha1.ResourceGroup)
		r.Spec.Subgroups = subgroups
		return kubeClient.Update(context.TODO(), rg)
	} else {
		r := rg.(*v1alpha1.ResourceGroup)
		r.Spec.Subgroups = subgroups
		return kubeClient.Update(context.TODO(), rg)
	}
}

func clientConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", path.Join(os.Getenv("HOME"), ".kube/config"))
}

func newclientSet(config *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

func newKubeClient(config *rest.Config) (client.Client, error) {
	return client.New(config, client.Options{})
}

func waitForDeployment(clientSet *kubernetes.Clientset, namespace, name string) error {
	return wait.PollImmediate(pollInterval, waitTimeout, func() (bool, error) {
		d, err := clientSet.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil && kubeerrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to get deployment %s: %v", name, err)
		}
		return d.Status.Replicas == d.Status.AvailableReplicas &&
			d.Status.Replicas == d.Status.ReadyReplicas, nil
	})
}

func waitForNamespaceToBeDeleted(clientSet *kubernetes.Clientset, name string) error {
	return wait.PollImmediate(pollInterval, waitTimeout, func() (bool, error) {
		_, err := clientSet.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if kubeerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
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

func RunMake(target string) error {
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
	cmd := exec.Command("bash", "-c", fmt.Sprintf("kubectl apply -f %s", file))
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
}

func waitForCRD(kubeClient client.Client, name string) error {
	crd := &v1.CustomResourceDefinition{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	crd.SetName(name)

	return wait.PollImmediate(pollInterval, waitTimeout, func() (bool, error) {
		err := kubeClient.Get(context.TODO(), client.ObjectKey{Name: crd.Name}, crd)
		if err != nil {
			return false, err
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == v1.Established {
				if condition.Status == v1.ConditionTrue {
					return true, nil
				}
			}
		}
		return false, nil
	})
}
