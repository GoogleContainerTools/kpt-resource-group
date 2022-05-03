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
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
	SetDefaultEventuallyTimeout(15 * time.Minute)
}

var kubeClient client.Client

var _ = BeforeSuite(func() {
	s := scheme.Scheme
	_ = v1alpha1.AddToScheme(s)
	_ = v1.AddToScheme(s)

	config, err := clientConfig()
	Expect(err).NotTo(HaveOccurred())
	clientSet, err = newclientSet(config)
	Expect(err).NotTo(HaveOccurred())

	By("Wait for resource group manager to be ready")
	err = waitForDeployment(clientSet, "resource-group-system", "resource-group-controller-manager")
	if err != nil {
		dumpEvents(clientSet, "resource-group-system")
	}
	Expect(err).NotTo(HaveOccurred())

	config, err = clientConfig()
	Expect(err).NotTo(HaveOccurred())
	clientSet, err = newclientSet(config)
	Expect(err).NotTo(HaveOccurred(), "failed to create kubernetes client: %v", err)
	kubeClient, err = newKubeClient(config)
	Expect(err).NotTo(HaveOccurred())
	By("Create test namespace")
	_, err = clientSet.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Print the resource-group-controller logs")
	log, _ := GetLogs()
	fmt.Printf("%s", string(log))
	By("Delete test namespace")
	foreground := metav1.DeletePropagationForeground
	err := clientSet.CoreV1().Namespaces().Delete(context.TODO(), testNamespace, metav1.DeleteOptions{
		PropagationPolicy: &foreground,
	})
	Expect(err).NotTo(HaveOccurred())

	err = waitForNamespaceToBeDeleted(clientSet, testNamespace)
	Expect(err).NotTo(HaveOccurred())
})
