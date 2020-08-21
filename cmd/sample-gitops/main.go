package main

import (
	"context"
	"fmt"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/labstack/echo"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/common"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/pkg/gitdir"
	"github.com/weaveworks/libgitops/pkg/logs"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/transaction"
	githubpr "github.com/weaveworks/libgitops/pkg/storage/transaction/pullrequest/github"
)

var (
	identityFlag    = pflag.String("identity-file", "", "Path to where the SSH private key is")
	authorNameFlag  = pflag.String("author-name", defaultAuthorName, "Author name for Git commits")
	authorEmailFlag = pflag.String("author-email", defaultAuthorEmail, "Author email for Git commits")
	gitURLFlag      = pflag.String("git-url", "", "HTTPS Git URL; where the Git repository is, e.g. https://github.com/luxas/ignite-gitops")
	prAssigneeFlag  = pflag.StringSlice("pr-assignees", nil, "What user logins to assign for the created PR. The user must have pull access to the repo.")
	prMilestoneFlag = pflag.String("pr-milestone", "", "What milestone to tag the PR with")
)

const (
	sshKnownHostsFile = "~/.ssh/known_hosts"

	defaultAuthorName  = "Weave libgitops"
	defaultAuthorEmail = "support@weave.works"
)

func main() {
	// Parse the version flag
	common.ParseVersionFlag()

	// Run the application
	if err := run(*identityFlag, *gitURLFlag, os.Getenv("GITHUB_TOKEN"), *authorNameFlag, *authorEmailFlag); err != nil {
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

func run(identityFile, gitURL, ghToken, authorName, authorEmail string) error {
	// Validate parameters
	if len(identityFile) == 0 {
		return fmt.Errorf("--identity-file is required")
	}
	if len(gitURL) == 0 {
		return fmt.Errorf("--git-url is required")
	}
	if len(ghToken) == 0 {
		return fmt.Errorf("--github-token is required")
	}
	if len(authorName) == 0 {
		return fmt.Errorf("--author-name is required")
	}
	if len(authorEmail) == 0 {
		return fmt.Errorf("--author-email is required")
	}

	// Read the identity and known_hosts files
	identityContent, err := expandAndRead(identityFile)
	if err != nil {
		return err
	}
	knownHostsContent, err := expandAndRead(sshKnownHostsFile)
	if err != nil {
		return err
	}

	// Parse the HTTPS clone URL
	repoRef, err := gitprovider.ParseOrgRepositoryURL(gitURL)
	if err != nil {
		return err
	}

	// Create a new GitHub client using the given token
	ghClient, err := github.NewClient(github.WithOAuth2Token(ghToken))
	if err != nil {
		return err
	}

	// Authenticate to the GitDirectory using Git SSH
	authMethod, err := gitdir.NewSSHAuthMethod(identityContent, knownHostsContent)
	if err != nil {
		return err
	}

	// Construct the GitDirectory implementation which backs the storage
	gitDir, err := gitdir.NewGitDirectory(repoRef, gitdir.GitDirectoryOptions{
		Branch:     "master",
		Interval:   10 * time.Second,
		AuthMethod: authMethod,
	})
	if err != nil {
		return err
	}

	// Create a new PR provider for the GitStorage
	prProvider, err := githubpr.NewGitHubPRProvider(ghClient)
	if err != nil {
		return err
	}
	// Create a new GitStorage using the GitDirectory, PR provider, and Serializer
	gitStorage, err := transaction.NewGitStorage(gitDir, prProvider, scheme.Serializer)
	if err != nil {
		return err
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.InfoLevel)

	watchStorage, err := watch.NewManifestStorage(gitDir.Dir(), scheme.Serializer)
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

	e.GET("/git/", func(c echo.Context) error {
		objs, err := gitStorage.List(storage.NewKindKey(common.CarGVK))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, objs)
	})

	e.PUT("/git/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		objKey := common.CarKeyForName(name)
		err := gitStorage.Transaction(context.Background(), fmt.Sprintf("%s-update-", name), func(ctx context.Context, s storage.Storage) (transaction.CommitResult, error) {

			// Update the status of the car
			if err := common.SetNewCarStatus(s, objKey); err != nil {
				return nil, err
			}

			return &transaction.GenericPullRequestResult{
				CommitResult: &transaction.GenericCommitResult{
					AuthorName:  authorName,
					AuthorEmail: authorEmail,
					Title:       "Update Car speed",
					Description: "We really need to sync this state!",
				},
				Labels:    []string{"user/bot", "actuator/libgitops", "kind/status-update"},
				Assignees: *prAssigneeFlag,
				Milestone: *prMilestoneFlag,
			}, nil
		})
		if err != nil {
			return err
		}

		return c.String(200, "OK!")
	})

	return common.StartEcho(e)
}
