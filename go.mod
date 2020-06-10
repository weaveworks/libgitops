module github.com/weaveworks/libgitops

go 1.12

require (
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/fluxcd/toolkit v0.0.1-beta.2
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-logfmt/logfmt v0.4.0 // indirect
	github.com/go-openapi/spec v0.19.5
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.2.9 // indirect
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.5
	golang.org/x/sys v0.0.0-20200323222414-85ca7c5b95cd
	k8s.io/apimachinery v0.18.2
	k8s.io/kube-openapi v0.0.0-20200427153329-656914f816f9
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
