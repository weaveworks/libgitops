module github.com/weaveworks/gitops-toolkit

go 1.12

require (
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/go-openapi/spec v0.19.2
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d // indirect
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/json-iterator/go v1.1.7 // indirect
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.2.9 // indirect
	github.com/pkg/errors v0.8.1
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.3
	github.com/whilp/git-urls v0.0.0-20160530060445-31bac0d230fa
	golang.org/x/net v0.0.0-20190812203447-cdfb69ac37fc // indirect
	golang.org/x/sys v0.0.0-20190616124812-15dcb6c0061f
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/klog v0.4.0 // indirect
	k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	sigs.k8s.io/yaml v1.1.0
)

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
