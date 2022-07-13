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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"kpt.dev/resourcegroup/controllers/resourcemap"
	"kpt.dev/resourcegroup/controllers/typeresolver"
)

const contextRootControllerKey = contextKey("root-controller")

var c client.Client
var ctx context.Context

var _ = Describe("Root Reconciler", func() {
	var reconcilerKpt *reconciler
	var namespace = metav1.NamespaceDefault

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		mgr, err := manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		reconcilerKpt, err = NewReconciler(mgr)
		Expect(err).ShouldNot(HaveOccurred())

		logger := reconcilerKpt.log.WithValues("Controller", "Root")
		ctx = context.WithValue(context.TODO(), contextRootControllerKey, logger)
		resolver, err := typeresolver.NewTypeResolver(mgr, logger)
		Expect(err).ShouldNot(HaveOccurred())
		reconcilerKpt.resolver = resolver

		StartTestManager(mgr)
		time.Sleep(10 * time.Second)
	})

	AfterEach(func() {
	})

	Describe("Root Controller Reconcile", func() {
		BeforeEach(func() {
			reconcilerKpt.resMap = resourcemap.NewResourceMap()
			reconcilerKpt.channel = make(chan event.GenericEvent)
		})

		It("creat, update and delete ResourceGroups in both configsync and kpt groups", func() {
			resources := []v1alpha1.ObjMetadata{}

			resourceGroupKpt := &v1alpha1.ResourceGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "group0",
					Namespace: namespace,
				},
				Spec: v1alpha1.ResourceGroupSpec{
					Resources: resources,
				},
			}

			err := c.Create(ctx, resourceGroupKpt)
			Expect(err).NotTo(HaveOccurred())

			// Create triggers an reconcilation,
			// wait until the reconcilation ends.
			time.Sleep(time.Second)
			Expect(reconcilerKpt.watches.Len()).Should(Equal(0))
			Expect(reconcilerKpt.resMap).ShouldNot(BeNil())

			// The resmap should be updated correctly
			request := ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: namespace, Name: "group0"},
			}
			Expect(reconcilerKpt.resMap.HasResgroup(request.NamespacedName)).Should(Equal(true))

			// There should be one event pushed to the channel.
			var e event.GenericEvent
			go func() {
				e = <-reconcilerKpt.channel
			}()
			time.Sleep(time.Second)
			Expect(e.Object.GetName()).Should(Equal("group0"))

			// update the Resourcegroup
			resources = []v1alpha1.ObjMetadata{
				{
					Name:      "statefulset",
					Namespace: namespace,
					GroupKind: v1alpha1.GroupKind{
						Group: "apps",
						Kind:  "StatefulSet",
					},
				},
				{
					Name:      "deployment",
					Namespace: namespace,
					GroupKind: v1alpha1.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
				},
				{
					Name:      "deployment-2",
					Namespace: namespace,
					GroupKind: v1alpha1.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
				},
				{
					Name:      "daemonset",
					Namespace: namespace,
					GroupKind: v1alpha1.GroupKind{
						Group: "apps",
						Kind:  "DaemonSet",
					},
				},
			}

			resourceGroupKpt.Spec = v1alpha1.ResourceGroupSpec{
				Resources: resources,
			}
			// Update the resource group
			err = c.Update(ctx, resourceGroupKpt)
			Expect(err).NotTo(HaveOccurred())

			// The update triggers another reconcile
			// wait until it ends.
			time.Sleep(time.Second)
			Expect(reconcilerKpt.watches.Len()).Should(Equal(3))
			Expect(reconcilerKpt.resMap).ShouldNot(BeNil())

			// The resmap should be updated correctly
			Expect(reconcilerKpt.resMap.HasResgroup(request.NamespacedName)).Should(Equal(true))
			for _, resource := range resources {
				Expect(reconcilerKpt.resMap.HasResource(resource)).Should(Equal(true))
				Expect(reconcilerKpt.resMap.Get(resource)).Should(Equal([]types.NamespacedName{request.NamespacedName}))
			}

			// The watchmap should be updated correctly
			for _, r := range []*reconciler{reconcilerKpt} {
				watched := r.watches.IsWatched(schema.GroupVersionKind{
					Group: "apps", Version: "v1", Kind: "Deployment"})
				Expect(watched).Should(Equal(true))
				watched = r.watches.IsWatched(schema.GroupVersionKind{
					Group: "apps", Version: "v1", Kind: "StatefulSet"})
				Expect(watched).Should(Equal(true))
				watched = r.watches.IsWatched(schema.GroupVersionKind{
					Group: "apps", Version: "v1", Kind: "DaemonSet"})
				Expect(watched).Should(Equal(true))
				Expect(r.watches.Len()).Should(Equal(3))
			}

			// There should be one event pushed to the channel.
			go func() { e = <-reconcilerKpt.channel }()
			time.Sleep(time.Second)
			Expect(e.Object.GetName()).Should(Equal("group0"))

			// Delete the resource group
			err = c.Delete(ctx, resourceGroupKpt)
			Expect(err).NotTo(HaveOccurred())

			// The delete triggers another reconcile
			// wait until it ends.
			time.Sleep(2 * time.Second)
			//Expect(reconcilerKpt.watches.Len()).Should(Equal(0))
			Expect(reconcilerKpt.resMap).ShouldNot(BeNil())

			// The resmap should be updated correctly
			// It doesn't contain any resourcegroup or resource
			Expect(reconcilerKpt.resMap.HasResgroup(request.NamespacedName)).Should(Equal(false))
			Expect(reconcilerKpt.resMap)
			for _, resource := range resources {
				Expect(reconcilerKpt.resMap.HasResource(resource)).Should(Equal(false))
			}

			// There should be one event pushed to the channel.
			go func() { e = <-reconcilerKpt.channel }()
			time.Sleep(time.Second)
			Expect(e.Object.GetName()).Should(Equal("group0"))
		})
	})

})
