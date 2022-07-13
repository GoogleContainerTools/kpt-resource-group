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
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

// Asserter is an Equal matcher with custom comparison options
var Asserter = testutil.NewAsserter(
	cmpopts.EquateErrors(),
	conditionComparer(),
)

// conditionComparer returns a Comparer for Conditions that ignores
// LastTransitionTime and allows substring matching of Message.
// Message substring matching is symmetric in order to satisfy Comparer
// requirements
func conditionComparer() cmp.Option {
	return cmp.Comparer(func(x, y v1alpha1.Condition) bool {
		return x.Type == y.Type &&
			x.Status == y.Status &&
			x.Reason == y.Reason &&
			(x.Message == y.Message ||
				strings.Contains(x.Message, y.Message) ||
				strings.Contains(y.Message, x.Message))
	})
}
