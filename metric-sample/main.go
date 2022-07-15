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

package main

import (
	"context"
	"os"
	"time"

	"github.com/go-logr/glogr"
	"k8s.io/apimachinery/pkg/types"
	"kpt.dev/resourcegroup/controllers/metrics"
)

var (
	logger = glogr.New().WithName("metric")
)

func main() {
	// Register the OpenCensus views
	if err := metrics.RegisterReconcilerMetricsViews(); err != nil {
		logger.Error(err, "Failed to register OpenCensus views")
		os.Exit(1)
	}

	// Register the OC Agent exporter
	oce, err := metrics.RegisterOCAgentExporter()
	if err != nil {
		logger.Error(err, "Failed to register the OC Agent exporter")
		os.Exit(1)
	}

	defer func() {
		if err := oce.Stop(); err != nil {
			logger.Error(err, "Unable to stop the OC Agent exporter")
		}
	}()

	for {
		now := time.Now()
		time.Sleep(1 * time.Second)
		setMetrics(now)
	}
}

func setMetrics(start time.Time) {
	nn := types.NamespacedName{Namespace: "ns1", Name: "rg1"}
	ctx := context.TODO()
	metrics.RecordReadyResourceCount(ctx, nn, 20)
	metrics.RecordCRDCount(ctx, nn, 2)
	metrics.RecordClusterScopedResourceCount(ctx, nn, 3)
	metrics.RecordKCCResourceCount(ctx, nn, 12)
	metrics.RecordNamespaceCount(ctx, nn, 1)
	metrics.RecordReadyResourceCount(ctx, nn, 12)
	metrics.RecordReconcileDuration(ctx, "reason", start)
	metrics.RecordResourceCount(ctx, nn, 15)
	metrics.RecordResourceGroupTotal(ctx, 2)
}
