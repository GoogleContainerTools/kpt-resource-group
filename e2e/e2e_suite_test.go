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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
	SetDefaultEventuallyTimeout(15 * time.Minute)
}

var (
	config     *rest.Config
	clientSet  *kubernetes.Clientset
	mapper     meta.ResettableRESTMapper
	kubeClient client.Client
)

var _ = BeforeSuite(func() {
	s := scheme.Scheme
	_ = v1alpha1.AddToScheme(s)
	_ = apiextensionsv1.AddToScheme(s)

	var err error
	config, err = clientConfig()
	Expect(err).NotTo(HaveOccurred())
	clientSet, err = newClientSet(config)
	Expect(err).NotTo(HaveOccurred())
	mapper = newRESTMapper(clientSet)
	kubeClient, err = newKubeClient(config, mapper)
	Expect(err).NotTo(HaveOccurred())

	By("Wait for resource group manager to be ready")
	err = InterceptGomegaFailure(func() {
		waitForDeploymentCurrent(kubeClient, "resource-group-system", "resource-group-controller-manager")
	})
	if err != nil {
		dumpEvents(clientSet, "resource-group-system")
		Fail(err.Error())
	}

	err = InterceptGomegaFailure(func() {
		By("Delete test namespace")
		deleteNamespace(kubeClient, testNamespace)

		By("Create test namespace")
		applyNamespace(kubeClient, testNamespace)
	})
	if err != nil {
		dumpEvents(clientSet, testNamespace)
		Fail(err.Error())
	}
})

// TODO: Replace with CurrentSpecReport() in AfterSuite after upgrading to Ginkgo v2
var suiteFailed = false
var _ = AfterEach(func() {
	suiteFailed = suiteFailed || CurrentGinkgoTestDescription().Failed
})

var _ = AfterSuite(func() {
	if suiteFailed {
		By("Print the resource-group-controller logs")
		log, _ := GetLogs()
		fmt.Fprintf(GinkgoWriter, "%s", string(log))
	}

	err := InterceptGomegaFailure(func() {
		By("Delete test namespace")
		deleteNamespace(kubeClient, testNamespace)
	})
	if err != nil {
		dumpEvents(clientSet, testNamespace)
		Fail(err.Error())
	}
})
