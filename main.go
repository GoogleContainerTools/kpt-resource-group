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
	"flag"
	"os"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"kpt.dev/resourcegroup/apis/kpt.dev/v1alpha1"
	"kpt.dev/resourcegroup/controllers/log"
	ocmetrics "kpt.dev/resourcegroup/controllers/metrics"
	"kpt.dev/resourcegroup/controllers/profiler"
	"kpt.dev/resourcegroup/controllers/resourcegroup"
	"kpt.dev/resourcegroup/controllers/resourcemap"
	"kpt.dev/resourcegroup/controllers/root"
	"kpt.dev/resourcegroup/controllers/typeresolver"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = v1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme

	_ = apiextensionsv1.AddToScheme(scheme)
}

func main() {
	log.InitFlags()

	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	profiler.Service()

	// Register the OpenCensus views
	if err := ocmetrics.RegisterReconcilerMetricsViews(); err != nil {
		setupLog.Error(err, "Failed to register OpenCensus views")
		os.Exit(1)
	}

	// Register the OC Agent exporter
	oce, err := ocmetrics.RegisterOCAgentExporter()
	if err != nil {
		setupLog.Error(err, "Failed to register the OC Agent exporter")
		os.Exit(1)
	}

	defer func() {
		if err := oce.Stop(); err != nil {
			setupLog.Error(err, "Unable to stop the OC Agent exporter")
		}
	}()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "413d8c8e.gke.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	logger := ctrl.Log.WithName("controllers")

	for _, group := range []string{root.KptGroup} {
		registerControllersForGroup(mgr, logger, group)
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func registerControllersForGroup(mgr ctrl.Manager, logger logr.Logger, group string) {
	// channel is watched by ResourceGroup controller.
	// The Root controller pushes events to it and
	// the ResourceGroup controller consumes events.
	channel := make(chan event.GenericEvent)

	setupLog.Info("adding the type resolver")
	resolver, err := typeresolver.NewTypeResolver(mgr, logger.WithName("TypeResolver"))
	if err != nil {
		setupLog.Error(err, "unable to set up the type resolver")
		os.Exit(1)
	}
	resolver.Refresh()

	setupLog.Info("adding the Root controller for group " + group)
	resMap := resourcemap.NewResourceMap()
	if err := root.NewController(mgr, channel, logger.WithName("Root"), resolver, group, resMap); err != nil {
		setupLog.Error(err, "unable to create the root controller for group "+group)
		os.Exit(1)
	}

	setupLog.Info("adding the ResourceGroup controller for group " + group)
	if err := resourcegroup.NewRGController(mgr, channel, logger.WithName(v1alpha1.ResourceGroupKind), resolver, resMap, resourcegroup.DefaultDuration); err != nil {
		setupLog.Error(err, "unable to create the ResourceGroup controller")
		os.Exit(1)
	}
}
