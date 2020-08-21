package main

import (
	"bytes"
	"fmt"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
	"net/http"
	"os"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/cmd/common"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/v1alpha1"
	"github.com/weaveworks/libgitops/pkg/logs"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
)

const (
	ManifestDir = "/tmp/libgitops/manifest"
	WatchDir    = "/tmp/libgitops/watch"
)

func main() {
	// Parse the version flag
	common.ParseVersionFlag()

	// Run the application
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// Create the manifest and watch directories
	if err := os.MkdirAll(ManifestDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(WatchDir, 0755); err != nil {
		return err
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.TraceLevel)

	plainStorage := storage.NewGenericStorage(
		storage.NewGenericRawStorage(ManifestDir, v1alpha1.SchemeGroupVersion, serializer.ContentTypeYAML),
		scheme.Serializer,
		[]runtime.IdentifierFactory{runtime.Metav1NameIdentifier},
	)
	defer func() { _ = plainStorage.Close() }()

	watchStorage, err := watch.NewManifestStorage(WatchDir, scheme.Serializer)
	if err != nil {
		return err
	}
	defer func() { _ = watchStorage.Close() }()

	updates := make(chan update.Update, 4096)
	watchStorage.SetUpdateStream(updates)

	go func() {
		for upd := range updates {
			logrus.Infof("Got %s update for: %v %v", upd.Event, upd.PartialObject.GetObjectKind().GroupVersionKind(), upd.PartialObject.GetObjectMeta())
		}
	}()

	e := common.NewEcho()

	e.GET("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj, err := plainStorage.Get(common.CarKeyForName(name))
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), obj); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.POST("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := plainStorage.Create(common.NewCar(name)); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	e.PUT("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := common.SetNewCarStatus(plainStorage, common.CarKeyForName(name)); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	e.GET("/watch/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj, err := watchStorage.Get(common.CarKeyForName(name))
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), obj); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.PUT("/watch/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := common.SetNewCarStatus(plainStorage, common.CarKeyForName(name)); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	return common.StartEcho(e)
}
