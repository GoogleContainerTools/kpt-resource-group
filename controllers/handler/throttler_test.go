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

package handler

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
)

var _ = Describe("Unit tests", func() {
	Describe("Throttler", func() {
		u := &v1alpha1.ResourceGroup{}
		u.SetName("group")
		u.SetNamespace("ns")

		// Push an event to channel
		genericE := event.GenericEvent{
			Object: u,
		}

		u2 := &v1alpha1.ResourceGroup{}
		u2.SetName("group2")
		u2.SetNamespace("ns")

		genericE2 := event.GenericEvent{
			Object: u2,
		}

		It("Add one event", func() {
			throttler := NewThrottler(time.Second)
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

			throttler.Generic(genericE, queue)

			_, found := throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(true))
			time.Sleep(2 * time.Second)
			_, found = throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(false))

			// The queue should contain only one event
			Expect(queue.Len()).Should(Equal(1))
		})

		It("multiple events for the same object are throttled to one", func() {
			// Set the duration to 5 seconds
			throttler := NewThrottler(5 * time.Second)
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

			// Call the event handler three times for the same event
			throttler.Generic(genericE, queue)
			throttler.Generic(genericE, queue)
			throttler.Generic(genericE, queue)

			// After 3 seconds, still within the duration, the event can
			// be found in the mapping
			time.Sleep(3 * time.Second)
			_, found := throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(true))

			// After 3 + 3 seconds, the duration ends, the event
			// is removed from the mapping
			time.Sleep(3 * time.Second)
			_, found = throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(false))

			// The queue should contain only one event
			Expect(queue.Len()).Should(Equal(1))
		})

		It("events for multiple objects are kept", func() {
			// Set the duration to 5 seconds
			throttler := NewThrottler(5 * time.Second)
			queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

			// Call the event handler to push two events
			throttler.Generic(genericE, queue)
			throttler.Generic(genericE2, queue)
			throttler.Generic(genericE, queue)
			throttler.Generic(genericE2, queue)

			// After 3 seconds, still within the duration, the events can
			// be found in the mapping
			time.Sleep(3 * time.Second)
			_, found := throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(true))
			_, found = throttler.mapping[types.NamespacedName{
				Name:      "group2",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(true))

			// After 3 + 3 seconds, the duration ends, the events
			// is removed from the mapping
			time.Sleep(3 * time.Second)
			_, found = throttler.mapping[types.NamespacedName{
				Name:      "group",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(false))
			_, found = throttler.mapping[types.NamespacedName{
				Name:      "group2",
				Namespace: "ns",
			}]
			Expect(found).Should(Equal(false))

			// The queue should contain two events
			Expect(queue.Len()).Should(Equal(2))
		})
	})
})
