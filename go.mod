module github.com/weaveworks/libgitops

go 1.14

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible

require (
	github.com/fluxcd/toolkit v0.0.1-beta.2
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-openapi/spec v0.19.8
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/onsi/ginkgo v1.12.0 // indirect
	github.com/onsi/gomega v1.9.0 // indirect
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/sys v0.0.0-20200610111108-226ff32320da
	k8s.io/apimachinery v0.18.5
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/kustomize/kyaml v0.1.11
	sigs.k8s.io/yaml v1.2.0
)
