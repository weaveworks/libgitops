package transaction

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/gitdir"
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/util/watcher"
)

var excludeDirs = []string{".git"}

func NewGitStorage(gitDir *gitdir.GitDirectory, ser serializer.Serializer) (TransactionStorage, error) {
	// Make sure the repo is cloned
	gitDir.StartCheckoutLoop()

	raw := storage.NewGenericMappedRawStorage(gitDir.Dir())
	s := storage.NewGenericStorage(raw, ser, []runtime.IdentifierFactory{runtime.Metav1NameIdentifier})

	gitStorage := &GitStorage{
		ReadStorage: s,
		s: s,
		raw: raw,
		gitDir: gitDir,
	}
	// Do a first resync now
	if err := gitStorage.resync(); err != nil {
		return nil, err
	}

	return gitStorage, nil
}

type GitStorage struct {
	storage.ReadStorage

	s storage.Storage
	raw storage.MappedRawStorage
	gitDir *gitdir.GitDirectory
}

func (s *GitStorage) Resume() {
	s.gitDir.Resume()
}

func (s *GitStorage) Suspend() {
	s.gitDir.Suspend()
}

func (s *GitStorage) Pull(ctx context.Context) error {
	return s.gitDir.Pull(ctx)
}

func (s *GitStorage) resyncLoop() {
	go func() {
		for {
			if commit, ok := <-s.gitDir.CommitChannel(); ok {
				logrus.Debugf("GitStorage: Got info about commit %q, resyncing...", commit)
				if err := s.resync(); err != nil {
					logrus.Errorf("GitStorage: Got resync error: %v", err)
				}
			}
		}
	}()
}

func (s *GitStorage) resync() error {
	mappings, err := computeMappings(s.gitDir.Dir(), s.s)
	if err != nil {
		return err
	}
	logrus.Debugf("Rewriting the mappings to %v", mappings)
	s.raw.SetMappings(mappings)
	return nil
}

func (s *GitStorage) Transaction(ctx context.Context, streamName string, fn TransactionFunc) error {
	if err := s.Pull(ctx); err != nil {
		return err
	}
	s.Suspend()
	defer s.Resume()
	defer s.gitDir.ToMainBranch() // TODO ordering

	if err := s.gitDir.NewBranch(streamName); err != nil {
		return err
	}
	spec, err := fn(ctx, s.s)
	if err != nil {
		return err
	}
	return s.gitDir.Commit(ctx, *spec.AuthorName, *spec.AuthorEmail, spec.Message)
}

func computeMappings(dir string, s storage.Storage) (map[storage.ObjectKey]string, error) {
	validExts := make([]string, 0, len(storage.ContentTypes))
	for ext := range storage.ContentTypes {
		validExts = append(validExts, ext)
	}

	files, err := watcher.WalkDirectoryForFiles(dir, validExts, excludeDirs)
	if err != nil {
		return nil, err
	}
	
	m := map[storage.ObjectKey]string{}
	for _, file := range files {
		partObjs, err := storage.DecodePartialObjects(serializer.FromFile(file), s.Serializer().Scheme(), false, nil)
		if err != nil {
			logrus.Errorf("couldn't decode %q into a partial object: %v", file, err)
			continue
		}
		key, err := s.ObjectKeyFor(partObjs[0])
		if err != nil {
			logrus.Errorf("couldn't get objectkey for partial object: %v", err)
			continue
		}
		logrus.Debugf("Adding mapping between %s and %q", key, file)
		m[key] = file
	}
	return m, nil
}