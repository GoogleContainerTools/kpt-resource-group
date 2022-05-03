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

package root

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go-logr/glogr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"kpt.dev/resourcegroup/controllers/resourcemap"
	"kpt.dev/resourcegroup/controllers/watch"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(glogr.New())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	s := scheme.Scheme
	err = v1alpha1.AddToScheme(s)
	Expect(err).NotTo(HaveOccurred())
	err = v1.AddToScheme(s)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: s})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager) {
	go func() {
		Expect(mgr.Start(context.Background())).NotTo(HaveOccurred())
	}()
}

func NewReconciler(mgr manager.Manager) (*reconciler, error) {
	resmap := resourcemap.NewResourceMap()
	watches, err := watch.NewManager(mgr.GetConfig(), resmap, nil, nil)
	if err != nil {
		return nil, err
	}
	r := &reconciler{
		Client:  mgr.GetClient(),
		cfg:     mgr.GetConfig(),
		log:     ctrl.Log.WithName("controllers").WithName("Root"),
		resMap:  resmap,
		watches: watches,
	}
	obj := &v1alpha1.ResourceGroup{}
	_, err = ctrl.NewControllerManagedBy(mgr).
		For(obj).
		Build(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}
