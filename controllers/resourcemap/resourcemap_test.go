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

package resourcemap

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Util tests", func() {
	res1 := resource{
		Namespace: "ns1",
		Name:      "res1",
		GroupKind: v1alpha1.GroupKind{
			Group: "group1",
			Kind:  "service",
		},
	}

	res2 := resource{
		Namespace: "ns1",
		Name:      "res2",
		GroupKind: v1alpha1.GroupKind{
			Group: "group1",
			Kind:  "service",
		},
	}

	res3 := resource{
		Namespace: "ns1",
		Name:      "res3",
		GroupKind: v1alpha1.GroupKind{
			Group: "group1",
			Kind:  "service",
		},
	}

	gk := res1.GK()

	Describe("ResourceMap", func() {
		It("should reconcile resource group correctly", func() {
			resourceMap := NewResourceMap()
			Expect(resourceMap.IsEmpty()).Should(Equal(true))

			resgroup1 := types.NamespacedName{
				Namespace: "test-ns",
				Name:      "group1",
			}

			resgroup2 := types.NamespacedName{
				Namespace: "test-ns",
				Name:      "group2",
			}

			var gks []schema.GroupKind

			gks = resourceMap.Reconcile(context.TODO(), resgroup1, []resource{res1, res2}, false)
			Expect(gks).Should(Equal([]schema.GroupKind{gk}))
			Expect(len(resourceMap.gkToResources)).Should(Equal(1))
			Expect(resourceMap.gkToResources[gk].Len()).Should(Equal(2))
			Expect(resourceMap.gkToResources[gk].Has(res1)).Should(Equal(true))
			Expect(resourceMap.gkToResources[gk].Has(res2)).Should(Equal(true))

			Expect(resourceMap.IsEmpty()).Should(Equal(false))
			Expect(resourceMap.HasResource(res1)).Should(Equal(true))
			Expect(resourceMap.HasResource(res3)).Should(Equal(false))
			Expect(resourceMap.HasResgroup(resgroup1)).Should(Equal(true))
			Expect(resourceMap.HasResgroup(resgroup2)).Should(Equal(false))
			Expect(len(resourceMap.resgroupToResources)).Should(Equal(1))
			Expect(len(resourceMap.resToResgroups)).Should(Equal(2))
			Expect(resourceMap.resToResgroups[res1].Len()).Should(Equal(1))
			Expect(resourceMap.resToResgroups[res2].Len()).Should(Equal(1))
			Expect(resourceMap.resgroupToResources[resgroup1].Len()).Should(Equal(2))
			Expect(len(resourceMap.resToStatus)).Should(Equal(0))

			resourceMap.SetStatus(res1, &CachedStatus{Status: v1alpha1.Current})
			resourceMap.SetStatus(res2, &CachedStatus{Status: v1alpha1.InProgress})

			gks = resourceMap.Reconcile(context.TODO(), resgroup2, []resource{res1, res3}, false)
			Expect(len(gks)).Should(Equal(1))
			Expect(len(resourceMap.gkToResources)).Should(Equal(1))
			Expect(resourceMap.gkToResources[gk].Len()).Should(Equal(3))
			Expect(resourceMap.gkToResources[gk].Has(res3)).Should(Equal(true))
			Expect(len(resourceMap.resToStatus)).Should(Equal(2))
			cachedStatus := resourceMap.GetStatus(res1)
			Expect(cachedStatus.Status).Should(Equal(v1alpha1.Current))
			cachedStatus = resourceMap.GetStatus(res2)
			Expect(cachedStatus.Status).Should(Equal(v1alpha1.InProgress))
			cachedStatus = resourceMap.GetStatus(res3)
			Expect(cachedStatus).Should(BeNil())

			Expect(resourceMap.HasResource(res3)).Should(Equal(true))
			Expect(resourceMap.HasResgroup(resgroup2)).Should(Equal(true))
			Expect(len(resourceMap.resgroupToResources)).Should(Equal(2))
			Expect(len(resourceMap.resToResgroups)).Should(Equal(3))
			Expect(resourceMap.resToResgroups[res1].Len()).Should(Equal(2))
			Expect(resourceMap.resToResgroups[res2].Len()).Should(Equal(1))
			Expect(resourceMap.resToResgroups[res3].Len()).Should(Equal(1))

			gks = resourceMap.Reconcile(context.TODO(), resgroup1, []resource{res2}, false)
			Expect(len(gks)).Should(Equal(1))
			Expect(len(resourceMap.gkToResources)).Should(Equal(1))
			Expect(resourceMap.gkToResources[gk].Len()).Should(Equal(3))

			// res1 is still included in resgroup2
			Expect(resourceMap.HasResource(res1)).Should(Equal(true))
			Expect(len(resourceMap.resToResgroups)).Should(Equal(3))
			Expect(resourceMap.resToResgroups[res1].Len()).Should(Equal(1))

			// Set the resource set of resgroup1 to be empty
			gks = resourceMap.Reconcile(context.TODO(), resgroup1, []resource{}, false)
			Expect(len(gks)).Should(Equal(1))
			Expect(len(resourceMap.gkToResources)).Should(Equal(1))
			Expect(resourceMap.gkToResources[gk].Len()).Should(Equal(2))

			Expect(resourceMap.HasResgroup(resgroup1)).Should(Equal(true))
			Expect(resourceMap.HasResgroup(resgroup2)).Should(Equal(true))
			Expect(resourceMap.HasResource(res2)).Should(Equal(false))
			Expect(len(resourceMap.resgroupToResources)).Should(Equal(2))
			Expect(len(resourceMap.resToResgroups)).Should(Equal(2))
			Expect(len(resourceMap.resToStatus)).Should(Equal(1))

			// Set the resource set of resgroup2 to be empty
			gks = resourceMap.Reconcile(context.TODO(), resgroup2, []resource{}, false)
			Expect(len(gks)).Should(Equal(0))
			Expect(len(resourceMap.gkToResources)).Should(Equal(0))
			Expect(len(resourceMap.resgroupToResources)).Should(Equal(2))
			// delete resgroup1
			gks = resourceMap.Reconcile(context.TODO(), resgroup1, []resource{}, true)
			Expect(len(gks)).Should(Equal(0))

			// delete resgroup2
			gks = resourceMap.Reconcile(context.TODO(), resgroup2, []resource{}, true)
			Expect(len(gks)).Should(Equal(0))
			Expect(resourceMap.IsEmpty()).Should(Equal(true))
			Expect(len(resourceMap.resToStatus)).Should(Equal(0))
		})
	})

	Describe("diffResources", func() {
		It("should calculate diff of two resources correctly", func() {
			toAdd, toDelete := diffResources([]resource{}, []resource{})
			Expect(len(toAdd)).Should(Equal(0))
			Expect(len(toDelete)).Should(Equal(0))

			toAdd, toDelete = diffResources([]resource{}, []resource{res1})
			Expect(len(toAdd)).Should(Equal(1))
			Expect(len(toDelete)).Should(Equal(0))
			Expect(toAdd[0]).Should(Equal(res1))

			toAdd, toDelete = diffResources([]resource{res1, res2}, []resource{res1})
			Expect(len(toAdd)).Should(Equal(0))
			Expect(len(toDelete)).Should(Equal(1))
			Expect(toDelete[0]).Should(Equal(res2))

			toAdd, toDelete = diffResources([]resource{res1, res2}, []resource{res1, res3})
			Expect(len(toAdd)).Should(Equal(1))
			Expect(len(toDelete)).Should(Equal(1))
			Expect(toAdd[0]).Should(Equal(res3))
			Expect(toDelete[0]).Should(Equal(res2))
		})
	})
})
