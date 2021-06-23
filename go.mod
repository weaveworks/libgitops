module github.com/weaveworks/libgitops

go 1.15

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible

require (
	github.com/alcortesm/tgz v0.0.0-20161220082320-9c5fe88206d7 // indirect
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96 // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/fluxcd/go-git-providers v0.2.0
	github.com/fluxcd/pkg/ssh v0.2.0
	github.com/go-git/go-git/v5 v5.4.2
	github.com/go-openapi/spec v0.20.3
	github.com/go-openapi/strfmt v0.19.5 // indirect
	github.com/go-openapi/validate v0.19.8 // indirect
	github.com/google/btree v1.0.1
	github.com/google/go-github/v32 v32.1.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/markbates/pkger v0.17.1 // indirect
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/qri-io/starlib v0.4.2-0.20200213133954-ff2e8cd5ef8d // indirect
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	go.uber.org/tools v0.0.0-20190618225709-2cfd321de3ee // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/apimachinery v0.21.2
	k8s.io/kube-openapi v0.0.0-20210527164424-3c818078ee3d
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.1
	sigs.k8s.io/kustomize/kyaml v0.10.21
)
