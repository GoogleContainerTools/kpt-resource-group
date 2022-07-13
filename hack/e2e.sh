#!/bin/bash
# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

cd ${SCRIPT_ROOT}

GCP_PROJECT=resource-group-prow-test
GCP_ZONE=us-west1-b
# The cluster resource-group-e2e has Config Connector installed.
# So that it can test the kcc resources.
CLUSTER_NAME=resource-group-e2e
RG_DOCKER_IMAGE=gcr.io/${GCP_PROJECT}/resource-group
GIT_REV=$(git rev-parse HEAD)
export IMG=${RG_DOCKER_IMAGE}:${GIT_REV}

gcloud auth activate-service-account --key-file=/etc/service-account/service-account.json
gcloud auth configure-docker
gcloud config set project ${GCP_PROJECT}

# Get the kube config
gcloud container clusters get-credentials ${CLUSTER_NAME} --zone ${GCP_ZONE}
kubectl config set-context $(kubectl config current-context) --namespace=default

# Give current user admin access to allow creating test resources
count=$(kubectl get clusterrolebinding authenticated-admin --no-headers --ignore-not-found | wc -l)
if [[ "$count" -eq 0 ]]; then
  kubectl create clusterrolebinding authenticated-admin --clusterrole cluster-admin --user $(gcloud config get-value account)
fi

make e2e-test-gke
