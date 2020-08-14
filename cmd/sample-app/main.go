package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/v1alpha1"
	"github.com/weaveworks/libgitops/cmd/sample-app/version"
	"github.com/weaveworks/libgitops/pkg/gitdir"
	"github.com/weaveworks/libgitops/pkg/logs"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/transaction"
)

var (
	carGVK = v1alpha1.SchemeGroupVersion.WithKind("Car")

	identityFlag    = pflag.String("identity-file", "", "Path to where the SSH private key is")
	authorNameFlag  = pflag.String("author-name", defaultAuthorName, "Author name for Git commits")
	authorEmailFlag = pflag.String("author-email", defaultAuthorEmail, "Author email for Git commits")
	gitURLFlag      = pflag.String("git-url", "", "SSH Git URL; where the Git repository is, e.g. git@github.com:luxas/ignite-gitops.git")
)

const (
	ManifestDir       = "/tmp/libgitops/manifest"
	sshKnownHostsFile = "~/.ssh/known_hosts"

	defaultAuthorName  = "Weave libgitops"
	defaultAuthorEmail = "support@weave.works"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// Parse the version flag
	parseVersionFlag()

	// Run the application
	if err := run(*identityFlag, *gitURLFlag); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func expandAndRead(filePath string) ([]byte, error) {
	expandedPath, err := homedir.Expand(filePath)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(expandedPath)
}

func run(identityFile, gitURL string) error {
	if len(identityFile) == 0 {
		return fmt.Errorf("--identity-file is required")
	}
	if len(gitURL) == 0 {
		return fmt.Errorf("--git-url is required")
	}

	identityContent, err := expandAndRead(identityFile)
	if err != nil {
		return err
	}

	knownHostsContent, err := expandAndRead(sshKnownHostsFile)
	if err != nil {
		return err
	}

	// Construct the GitDirectory implementation which backs the storage
	gitDir, err := gitdir.NewGitDirectory(gitURL, gitdir.GitDirectoryOptions{
		Branch:                "master",
		Interval:              10 * time.Second,
		IdentityFileContent:   identityContent,
		KnownHostsFileContent: knownHostsContent,
	})
	if err != nil {
		return err
	}

	gitStorage, err := transaction.NewGitStorage(gitDir, scheme.Serializer)
	if err != nil {
		return err
	}

	// Create the manifest directory
	if err := os.MkdirAll(ManifestDir, 0755); err != nil {
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

	// Set up the echo server
	e := echo.New()
	e.Debug = true
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Welcome!")
	})

	e.GET("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		objKey := storage.NewObjectKey(storage.NewKindKey(carGVK), runtime.NewIdentifier("default/"+name))
		obj, err := plainStorage.Get(objKey)
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

		obj := &v1alpha1.Car{}
		obj.Name = name
		obj.Namespace = "default"
		obj.Spec.Brand = fmt.Sprintf("Acura-%03d", rand.Intn(1000))

		err := plainStorage.Set(obj)
		if err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	e.GET("/git/", func(c echo.Context) error {
		objs, err := gitStorage.List(storage.NewKindKey(carGVK))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, objs)
	})

	e.POST("/git/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		objKey := storage.NewObjectKey(storage.NewKindKey(carGVK), runtime.NewIdentifier("default/"+name))
		err := gitStorage.Transaction(context.Background(), "foo-branch-", func(ctx context.Context, s storage.Storage) (*transaction.CommitSpec, error) {
			obj, err := s.Get(objKey)
			if err != nil {
				return nil, err
			}
			car := obj.(*v1alpha1.Car)
			car.Status.Distance = rand.Uint64()
			car.Status.Speed = rand.Float64() * 100
			if err := s.Set(car); err != nil {
				return nil, err
			}

			return &transaction.CommitSpec{
				AuthorName:  authorNameFlag,
				AuthorEmail: authorEmailFlag,
				Message:     "Just updating some car status :)",
			}, nil
		})
		if err != nil {
			return err
		}

		return c.String(200, "OK!")
	})

	// Start the server
	go func() {
		if err := e.Start(":8888"); err != nil {
			e.Logger.Info("shutting down the server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	// Wait for interrupt signal to gracefully shutdown the application with a timeout of 10 seconds
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return e.Shutdown(ctx)
}

func parseVersionFlag() {
	var showVersion bool

	pflag.BoolVar(&showVersion, "version", showVersion, "Show version information and exit")
	pflag.Parse()
	if showVersion {
		fmt.Printf("sample-app version: %#v\n", version.Get())
		os.Exit(0)
	}
}
