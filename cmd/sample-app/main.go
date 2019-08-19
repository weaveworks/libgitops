package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/gitops-toolkit/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/gitops-toolkit/cmd/sample-app/client"
	"github.com/weaveworks/gitops-toolkit/pkg/filter"
	"github.com/weaveworks/gitops-toolkit/pkg/git/gitdir"
	"github.com/weaveworks/gitops-toolkit/pkg/logs"
	"github.com/weaveworks/gitops-toolkit/pkg/runtime"
	"github.com/weaveworks/gitops-toolkit/pkg/storage/cache"
	"github.com/weaveworks/gitops-toolkit/pkg/storage/manifest"
)

const ManifestDir = "/tmp/gitops-toolkit/manifest"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// Construct the GitDirectory implementation which backs the storage
	gitDir := gitdir.NewGitDirectory("https://github.com/luxas/ignite-gitops", "master", nil, 10*time.Second)

	// Wait for the repo to be cloned
	gitDir.WaitForClone()

	ms, err := manifest.NewManifestStorage(ManifestDir, scheme.Serializer)
	if err != nil {
		return err
	}
	Client := client.NewClient(cache.NewCache(ms))

	logs.Logger.SetLevel(logrus.DebugLevel)

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Welcome!")
	})

	e.GET("/:name", func(c echo.Context) error {
		kind := "car"
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set ref")
		}

		obj, err := Client.Dynamic(runtime.Kind(kind)).Find(filter.NewIDNameFilter(name))
		if err != nil {
			return err
		}
		content, err := scheme.Serializer.EncodeJSON(obj)
		if err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content)
	})

	e.POST("/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj := Client.Cars().New()
		obj.ObjectMeta.UID = runtime.UID("599615df99804ae8")
		obj.ObjectMeta.Name = name
		obj.Spec.Brand = "Acura"

		err := Client.Cars().Set(obj)
		if err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	return e.Start(":8080")
}

/*
car := &api.Car{
	Spec: api.CarSpec{
		Engine: "v8",
	},
	Status: api.CarStatus{
		VehicleStatus: api.VehicleStatus{
			Speed: 280.54,
			Distance: 532,
		},
		Persons: 2,
	},
}
car.SetName("mersu")
car.SetUID("1234")

if err := client.Cars().Set(car); err != nil {
		return err
	}

	carList, err := client.Cars().List()
	if err != nil {
		return err
	}
	for _, car := range carList {
		fmt.Println(*car)
	}
*/
