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

package resourcegroup

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
)

var _ = Describe("Util tests", func() {
	reconciling := v1alpha1.Condition{
		Type: v1alpha1.Reconciling,
	}
	stalled := v1alpha1.Condition{
		Type: v1alpha1.Stalled,
	}
	testCond1 := v1alpha1.Condition{
		Type: v1alpha1.ConditionType("hello"),
	}
	testCond2 := v1alpha1.Condition{
		Type: v1alpha1.ConditionType("world"),
	}
	Describe("adjustConditionOrder", func() {
		It("should order the reconciling and stalled conditions correctly", func() {
			conds := adjustConditionOrder([]v1alpha1.Condition{stalled, reconciling})
			Expect(len(conds)).Should(Equal(2))
			Expect(conds[0].Type).Should(Equal(v1alpha1.Reconciling))
			Expect(conds[1].Type).Should(Equal(v1alpha1.Stalled))
		})
		It("should handle empty condition slice correctly", func() {
			conds := adjustConditionOrder([]v1alpha1.Condition{})
			Expect(len(conds)).Should(Equal(2))
			Expect(conds[0].Type).Should(Equal(v1alpha1.Reconciling))
			Expect(conds[0].Status).Should(Equal(v1alpha1.UnknownConditionStatus))
			Expect(conds[1].Type).Should(Equal(v1alpha1.Stalled))
			Expect(conds[1].Status).Should(Equal(v1alpha1.UnknownConditionStatus))
		})
		It("should order the remaining conditions correctly", func() {
			conds := adjustConditionOrder([]v1alpha1.Condition{
				testCond2, stalled, testCond1, reconciling})
			Expect(len(conds)).Should(Equal(4))
			Expect(conds[0].Type).Should(Equal(v1alpha1.Reconciling))
			Expect(conds[1].Type).Should(Equal(v1alpha1.Stalled))
			Expect(conds[2].Type).Should(Equal(v1alpha1.ConditionType("hello")))
			Expect(conds[3].Type).Should(Equal(v1alpha1.ConditionType("world")))
		})
	})

	Describe("surfacing ownership test", func() {
		It("should return nil for empty inventory id and empty owning inventory", func() {
			id := ""
			c := ownershipCondition(id, "")
			Expect(c).Should(BeNil())
		})
		It("should return nil for matched inventory id and owning inventory", func() {
			id := "id"
			c := ownershipCondition(id, "id")
			Expect(c).Should(BeNil())
		})
		It("Should return unmatched message", func() {
			id := "id"
			c := ownershipCondition(id, "unmatched")
			Expect(c).ShouldNot(Equal(nil))
			Expect(c.Message).Should(ContainSubstring("owned by another"))
			Expect(c.Reason).Should(Equal(v1alpha1.OwnershipUnmatch))
			Expect(c.Status).Should(Equal(v1alpha1.TrueConditionStatus))
		})
		It("Should return not owned message", func() {
			id := "id"
			c := ownershipCondition(id, "")
			Expect(c).ShouldNot(Equal(nil))
			Expect(c.Message).Should(ContainSubstring("not owned by any"))
			Expect(c.Reason).Should(Equal(v1alpha1.OwnershipEmpty))
			Expect(c.Status).Should(Equal(v1alpha1.UnknownConditionStatus))
		})
	})

	Describe("surface the commit test", func() {
		It("should return empty when no annotations", func() {
			commit := getSourceHash(nil)
			Expect(commit).Should(BeEmpty())
		})
		It("should return empty when empty annotations", func() {
			commit := getSourceHash(map[string]string{})
			Expect(commit).Should(BeEmpty())
		})
		It("should return empty when annotation key doesn't exist", func() {
			commit := getSourceHash(map[string]string{"foo": "bar"})
			Expect(commit).Should(BeEmpty())
		})
		It("should return commit when annotation key exists", func() {
			annotations := map[string]string{
				"foo":                   "bar",
				SourceHashAnnotationKey: "1234567890",
			}
			commit := getSourceHash(annotations)
			Expect(commit).Should(Equal("1234567890"))
		})
	})
})
