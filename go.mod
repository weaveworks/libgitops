module github.com/weaveworks/libgitops

go 1.15

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible

require (
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/fluxcd/go-git-providers v0.2.0
	github.com/fluxcd/pkg/ssh v0.2.0
	github.com/go-git/go-git/v5 v5.4.2
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/spec v0.20.3
	github.com/google/btree v1.0.1
	github.com/google/go-github/v32 v32.1.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/otel v1.0.0-RC2
	go.opentelemetry.io/otel/exporters/jaeger v1.0.0-RC2
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.0.0-RC2
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.0.0-RC2
	go.opentelemetry.io/otel/sdk v1.0.0-RC2
	go.opentelemetry.io/otel/trace v1.0.0-RC2
	go.uber.org/atomic v1.7.0
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.18.1
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	k8s.io/apimachinery v0.21.3
	k8s.io/kube-openapi v0.0.0-20210527164424-3c818078ee3d
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/cluster-api v0.4.0
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/kustomize/kyaml v0.11.1-0.20210721155208-d6ce84604738
	sigs.k8s.io/yaml v1.2.0
)
