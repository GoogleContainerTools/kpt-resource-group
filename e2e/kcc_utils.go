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

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
)

var obj = map[string]interface{}{
	"apiVersion": "serviceusage.cnrm.cloud.google.com/v1beta1",
	"kind":       "Service",
	"metadata": map[string]interface{}{
		"name":      "pubsub.googleapis.com",
		"namespace": "default",
	},
}

func createKCCResource(kubeClient client.Client) v1alpha1.ObjMetadata {
	kccResource := &unstructured.Unstructured{Object: obj}
	err := kubeClient.Create(context.TODO(), kccResource)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create kcc resource %v", err)
	gk := kccResource.GroupVersionKind().GroupKind()
	return v1alpha1.ObjMetadata{
		GroupKind: v1alpha1.GroupKind{
			Group: gk.Group,
			Kind:  gk.Kind,
		},
		Namespace: kccResource.GetNamespace(),
		Name:      kccResource.GetName(),
	}
}

func deleteKCCResource(kubeClient client.Client) {
	kccResource := &unstructured.Unstructured{Object: obj}
	err := kubeClient.Delete(context.TODO(), kccResource)
	if err != nil {
		gomega.Expect(errors.IsNotFound(err)).Should(gomega.Equal(true))
	}
}

func makeKCCResourceStatus(resource v1alpha1.ObjMetadata) v1alpha1.ResourceStatus {
	status := v1alpha1.ResourceStatus{
		ObjMetadata: resource,
		Conditions: []v1alpha1.Condition{
			{
				Type:    "Ready",
				Status:  "False",
				Reason:  "UpdateFailed",
				Message: "Update call failed",
			},
		},
		Status: v1alpha1.InProgress,
	}
	return status
}
