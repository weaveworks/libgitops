module github.com/weaveworks/libgitops

go 1.14

replace (
	github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.0
)

require (
	github.com/fluxcd/go-git-providers v0.0.2
	github.com/fluxcd/toolkit v0.0.1-beta.2
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-openapi/spec v0.19.8
	github.com/google/go-github/v32 v32.1.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20200625001655-4c5254603344 // indirect
	golang.org/x/sys v0.0.0-20200812155832-6a926be9bd1d
	k8s.io/apimachinery v0.18.6
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/kustomize/kyaml v0.1.11
	sigs.k8s.io/yaml v1.2.0
)
