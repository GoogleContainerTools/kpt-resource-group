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

package typeresolver

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("resolver tests", func() {
	r := FakeResolver()

	Describe("Resolve GroupKind", func() {
		It("non existing type return false", func() {
			_, found := r.Resolve(schema.GroupKind{Group: "not.exist", Kind: "UnFound"})
			Expect(found).Should(Equal(false))
		})

		It("should have ConfigMap", func() {
			gvk, found := r.Resolve(schema.GroupKind{Group: "", Kind: "ConfigMap"})
			Expect(found).Should(Equal(true))
			Expect(gvk.Version).Should(Equal("v1"))
		})

		It("should have Deployment", func() {
			gvk, found := r.Resolve(schema.GroupKind{Group: "apps", Kind: "Deployment"})
			Expect(found).Should(Equal(true))
			Expect(gvk.Version).Should(Equal("v1"))
		})
	})
})
