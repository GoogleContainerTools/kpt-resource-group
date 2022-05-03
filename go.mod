module kpt.dev/resourcegroup

go 1.15

require (
	contrib.go.opencensus.io/exporter/ocagent v0.7.0
	github.com/go-logr/glogr v0.3.0
	github.com/go-logr/logr v0.4.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp v0.5.6
	github.com/onsi/ginkgo v1.16.2
	github.com/onsi/gomega v1.12.0
	github.com/pkg/errors v0.9.1
	go.opencensus.io v0.22.3
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781
	k8s.io/api v0.21.1
	k8s.io/apiextensions-apiserver v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	sigs.k8s.io/cli-utils v0.26.0
	sigs.k8s.io/controller-runtime v0.9.0-beta.5.0.20210524185538-7181f1162e79
	sigs.k8s.io/yaml v1.2.0
)
