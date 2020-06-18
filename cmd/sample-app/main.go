package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/cmd/sample-app/client"
	"github.com/weaveworks/libgitops/pkg/filter"
	"github.com/weaveworks/libgitops/pkg/gitdir"
	"github.com/weaveworks/libgitops/pkg/logs"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/cache"
	"github.com/weaveworks/libgitops/pkg/storage/manifest"
)

const ManifestDir = "/tmp/libgitops/manifest"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// Construct the GitDirectory implementation which backs the storage
	gitDir, err := gitdir.NewGitDirectory("https://github.com/luxas/ignite-gitops", gitdir.GitDirectoryOptions{
		Branch:   "master",
		Interval: 10 * time.Second,
	})
	if err != nil {
		return err
	}

	// Wait for the repo to be cloned
	if err := gitDir.WaitForClone(); err != nil {
		return err
	}

	// Create the manifest directory
	if err := os.MkdirAll(ManifestDir, 0755); err != nil {
		return err
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.DebugLevel)

	// Set up the ManifestStorage
	ms, err := manifest.NewManifestStorage(ManifestDir, scheme.Serializer)
	if err != nil {
		return err
	}
	defer func() { _ = ms.Close() }()
	Client := client.NewClient(cache.NewCache(ms))

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
		content, err := scheme.Serializer.Encoder(serializer.ContentTypeJSON).Encode(obj)
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
		obj.ObjectMeta.UID = "599615df99804ae8"
		obj.ObjectMeta.Name = name
		obj.Spec.Brand = "Acura"

		err := Client.Cars().Set(obj)
		if err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	// Start the server
	go func() {
		if err := e.Start(":8080"); err != nil {
			e.Logger.Info("shutting down the server")
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	// Wait for interrupt signal to gracefully shutdown the application with a timeout of 10 seconds
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return e.Shutdown(ctx)
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
