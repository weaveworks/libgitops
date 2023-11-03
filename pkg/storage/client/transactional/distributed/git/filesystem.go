package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-logr/logr"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/util/structerr"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ErrImmutableFilesystem = stringError("git clone is immutable; start a transaction to mutate")
)

type stringError string

func (s stringError) Error() string { return string(s) }

var (
	_ filesystem.Filesystem            = &Git{}
	_ transactional.TransactionManager = &Git{}
	_ distributed.Remote               = &Git{}
)

func New(ctx context.Context, repoRef gitprovider.RepositoryRef, opts ...Option) (*Git, error) {
	log := logr.FromContextOrDiscard(ctx)

	o := defaultOpts().ApplyOptions(opts)

	tmpDir, err := ioutil.TempDir("", "libgitops")
	if err != nil {
		return nil, err
	}
	log.V(2).Info("created temp directory to store Git clones in", "dir", tmpDir)
	tmpDirTyped := rootDir(tmpDir)

	transportType := gitprovider.TransportTypeHTTPS // default
	if o.AuthMethod != nil {
		// TODO: parse the URL instead
		transportType = o.AuthMethod.TransportType()
	}
	cloneURL := repoRef.GetCloneURL(transportType)

	cloneOpts := &git.CloneOptions{
		URL:          cloneURL,
		Auth:         o.AuthMethod,
		SingleBranch: true,
		NoCheckout:   true,
		//Depth:             1, // ref: https://github.com/go-git/go-git/issues/207
		RecurseSubmodules: 0,
		Progress:          nil,
		Tags:              git.NoTags,
	}
	if o.MainBranch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(o.MainBranch)
	}

	log.Info("cloning the repository", "repo-ref", repoRef)
	// Do a base clone to the temporary directory
	bareDir := filepath.Join(tmpDir, "root.git")
	repo, err := git.PlainCloneContext(ctx, bareDir, true, cloneOpts)
	// Handle errors
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("git clone operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("git clone was cancelled: %w", err)
	} else if err != nil {
		return nil, fmt.Errorf("git clone error: %v", err)
	}

	// Enable the uploadpack.allowReachableSHA1InWant option
	// http://git-scm.com/docs/git-config#Documentation/git-config.txt-uploadpackallowReachableSHA1InWant
	c, err := repo.Config()
	if err != nil {
		return nil, err
	}
	gitCfgBytes, _ := c.Marshal()
	log.V(2).Info("git config before", "git-config", string(gitCfgBytes))

	c.Raw.Section("uploadpack").SetOption("allowReachableSHA1InWant", "true")

	gitCfgBytes, _ = c.Marshal()
	log.V(2).Info("git config after", "git-config", string(gitCfgBytes))

	if err := repo.SetConfig(c); err != nil {
		return nil, err
	}

	// HEAD should be by default a symbolic reference to the main branch
	// TODO: Does this exist for a bare repository?
	r, err := repo.Head()
	if err != nil {
		return nil, err
	}
	mainBranch := string(r.Target())
	log.V(2).Info("got main branch", "main-branch", mainBranch)

	return &Git{
		Filesystem: filesystem.FromContext(&fileSystem{
			bareRepo:      repo,
			rootDir:       tmpDirTyped,
			defaultBranch: mainBranch,
		}),
		rootDir:       tmpDirTyped,
		bareDir:       bareDir,
		bareRepo:      repo,
		defaultBranch: mainBranch,
	}, nil
}

type rootDir string

func (d rootDir) gitDirFor(target commit.MutableTarget) string {
	return filepath.Join(string(d), string(target.UUID())) // +".git" TODO is this needed?
}

// TODO: Add a FilesystemFor(dir string) Filesystem method
type Git struct {
	filesystem.Filesystem
	rootDir
	bareDir       string
	bareRepo      *git.Repository
	defaultBranch string

	localClones   map[types.UID]*localClone
	localClonesMu *sync.Mutex
}

type localClone struct {
	repo   *git.Repository
	wt     *git.Worktree
	origin *git.Remote
	target commit.MutableTarget
}

func (g *Git) localCloneByUUID(uuid types.UID) (*localClone, bool) {
	// c.txsMu guards reads and writes of the c.txs map
	g.localClonesMu.Lock()
	defer g.localClonesMu.Unlock()

	// Check if information about a transaction on this branch exists.
	lc, ok := g.localClones[uuid]
	if ok {
		return lc, true
	}
	// if not, grow the localClones map by one and return it
	g.localClones[uuid] = &localClone{}
	return g.localClones[uuid], false
}

var _ structerr.StructError = &OngoingTransactionError{}

// Maybe move this to the transactional package?
type OngoingTransactionError struct {
	Target commit.MutableTarget
}

func (e *OngoingTransactionError) Error() string {
	msg := "cannot start a transaction with an UUID that already exists"
	if e.Target == nil {
		return msg
	}
	return fmt.Sprintf("%s: %s (base: %s, target: %s)", msg, e.Target.UUID(), e.Target.BaseCommit(), e.Target.DestBranch())
}

func (e *OngoingTransactionError) Is(err error) bool {
	_, ok := err.(*OngoingTransactionError)
	return ok
}

func (g *Git) Init(ctx context.Context, tx *transactional.TxInfo) error {
	target := tx.Target // TODO: Check for nil or not?

	lc, exists := g.localCloneByUUID(target.UUID())
	if exists {
		return &OngoingTransactionError{Target: target}
	}

	// Do a "git init", as per the instructions at
	// https://stackoverflow.com/questions/31278902/how-to-shallow-clone-a-specific-commit-with-depth-1
	var err error
	lc.repo, err = git.PlainInit(g.gitDirFor(target), false)
	if err != nil {
		return err
	}
	// Register the bare local clone as "origin"
	lc.origin, err = lc.repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{g.bareDir},
	})
	if err != nil {
		return err
	}
	// Fetch only this specific commit from the origin to HEAD, at depth 1
	refSpec := config.RefSpec(fmt.Sprintf("%s:refs/heads/HEAD", target.BaseCommit()))
	if err := lc.origin.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Depth:    1,
		Tags:     git.NoTags,
	}); err != nil {
		return err
	}
	// Now, check out the worktree
	lc.wt, err = lc.repo.Worktree()
	if err != nil {
		return err
	}
	// Create a new branch from the fetched commit, with the head branch name
	if err := lc.wt.Checkout(&git.CheckoutOptions{
		Hash:   *hashToGoGit(target.BaseCommit()),
		Branch: plumbing.NewBranchReferenceName(target.DestBranch()),
		Create: true,
	}); err != nil {
		return err
	}

	return nil
}

func (g *Git) Commit(ctx context.Context, tx *transactional.TxInfo, req commit.Request) error {
	log := logr.FromContextOrDiscard(ctx)
	target := tx.Target // TODO: Check for nil or not?

	lc, exists := g.localCloneByUUID(target.UUID())
	if exists {
		return stringError("nonexistent mutable target") // TODO
	}

	// TODO: Make sure this registers net-new files, too
	if err := lc.wt.AddGlob("."); err != nil {
		return err
	}

	t := req.Author().When()
	if t == nil {
		now := time.Now()
		t = &now
	}
	// TODO: This should be idempotent if the TransactionClient runs it over and over again
	newCommit, err := lc.wt.Commit(req.Message().String(), &git.CommitOptions{
		Author: &object.Signature{
			Name:  req.Author().Name(),
			Email: req.Author().Email(),
			When:  *t,
		},
		// TODO: SignKey
	})
	if err != nil {
		return err
	}
	log.V(2).Info("created commit with hash", "commit", newCommit.String())

	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", target.DestBranch(), target.DestBranch())
	if err := lc.origin.PushContext(ctx, &git.PushOptions{
		RefSpecs: []config.RefSpec{config.RefSpec(refSpec)},
	}); err != nil {
		return err // TODO: Error handling for context cancellations etc.
	}
	log.V(2).Info("pushed refspec", "refspec", refSpec)

	return nil
}

func (g *Git) Abort(ctx context.Context, tx *transactional.TxInfo) error {
	log := logr.FromContextOrDiscard(ctx)
	target := tx.Target // TODO: Check for nil or not?

	_, exists := g.localCloneByUUID(target.UUID())
	if !exists {
		return stringError("nonexistent mutable target") // TODO
	}

	// Removing the Git directory completely
	dir := g.gitDirFor(target)
	log.V(2).Info("removing local git directory clone", "dir", dir)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	// TODO: Shall this be done regardless of the os.RemoveAll error?
	g.localClonesMu.Lock()
	delete(g.localClones, target.UUID())
	g.localClonesMu.Unlock()
	return nil
}

func (g *Git) Pull(ctx context.Context) error {
	ref, ok := commit.GetRef(ctx)
	if !ok {
		return stringError("no commit.Ref given to Git.Pull")
	}
	var refName plumbing.ReferenceName
	tagMode := git.NoTags
	switch ref.Type() {
	case commit.RefTypeTag:
		refName = plumbing.NewTagReferenceName(ref.Target())
		tagMode = git.TagFollowing
	case commit.RefTypeBranch:
		refName = plumbing.NewBranchReferenceName(ref.Target())
	default:
		return fmt.Errorf("Git.Pull cannot support commit.Ref.Type = %s", ref.Type())
	}

	return g.bareRepo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refNameToSpec(refName)},
		Tags:       tagMode,
		// TODO: Do something with Depth here?
	})
}

func refNameToSpec(refName plumbing.ReferenceName) config.RefSpec {
	return config.RefSpec(fmt.Sprintf("%s:%s", refName, refName))
}

func (g *Git) Push(ctx context.Context) error {
	target, ok := commit.GetMutableTarget(ctx)
	if !ok {
		return stringError("no commit.MutableTarget given to Git.Push")
	}
	destRefName := plumbing.NewBranchReferenceName(target.DestBranch())
	return g.bareRepo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refNameToSpec(destRefName)},
	})
}

var _ filesystem.ContextFS = &fileSystem{}

type fileSystem struct {
	bareRepo *git.Repository
	rootDir
	defaultBranch string
}

func (f *fileSystem) ResolveRef(sr commit.Ref) (commit.Hash, error) {
	var h plumbing.Hash

	switch sr.Type() {
	case commit.RefTypeHash:
		c, err := f.bareRepo.CommitObject(plumbing.NewHash(sr.Target()))
		if err != nil {
			return nil, err
		}
		h = c.Hash
	case commit.RefTypeTag:
		t, err := f.bareRepo.Tag(sr.Target())
		if err != nil {
			return nil, err
		}
		h = t.Hash()
	default:
		ref := sr.Target()
		if sr.Type() == commit.RefTypeBranch {
			// Default the branch if left unset
			if ref == "" {
				// TODO: Get rid of this
				ref = f.defaultBranch
			}
			if sr.Before() != 0 {
				ref = fmt.Sprintf("%s~%d", sr.Target(), sr.Before())
			}
		}
		r, err := f.bareRepo.ResolveRevision(plumbing.Revision(ref))
		if err != nil {
			return nil, err
		}
		h = *r
	}
	return hashFromGoGit(h, sr), nil
}

func (f *fileSystem) GetRef(ctx context.Context) commit.Ref {
	ref, ok := commit.GetRef(ctx)
	if ok {
		return ref
	}
	return commit.AtBranch(f.defaultBranch)
}

func (f *fileSystem) RefResolver() commit.RefResolver { return f }

func (f *fileSystem) mutableFSFor(ctx context.Context, target commit.MutableTarget) filesystem.FS {
	return filesystem.NewOSFilesystem(f.gitDirFor(target)).WithContext(ctx)
}

func hashToGoGit(h commit.Hash) *plumbing.Hash {
	var ph plumbing.Hash
	copy(ph[:], h.Hash())
	return &ph
}

func hashFromGoGit(h plumbing.Hash, src commit.Ref) commit.Hash {
	return commit.SHA1(h, src)
}

func (f *fileSystem) hashFor(ctx context.Context) (*plumbing.Hash, error) {
	h, ok := commit.GetHash(ctx)
	if ok {
		return hashToGoGit(h), nil
	}
	// TODO: Use f.bareRepo.HEAD() here instead?
	return f.bareRepo.ResolveRevision(plumbing.Revision(f.defaultBranch))
}

func (f *fileSystem) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).MkdirAll(path, perm)
	}
	return ErrImmutableFilesystem
}

func (f *fileSystem) Remove(ctx context.Context, name string) error {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).Remove(name)
	}
	return ErrImmutableFilesystem
}

func (f *fileSystem) WriteFile(ctx context.Context, filename string, data []byte, perm os.FileMode) error {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).WriteFile(filename, data, perm)
	}
	return ErrImmutableFilesystem
}

// READ OPS

func (f *fileSystem) Open(ctx context.Context, name string) (fs.File, error) {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).Open(name)
	}
	h, err := f.hashFor(ctx)
	if err != nil {
		return nil, err
	}
	fi, t, err := f.stat(h, name)
	if err != nil {
		return nil, err
	}
	ff, err := t.File(name)
	if err != nil {
		return nil, err
	}
	rc, err := ff.Reader()
	if err != nil {
		return nil, err
	}
	return &fileWrapper{fi, rc}, nil
}

type fileWrapper struct {
	fi fs.FileInfo
	io.ReadCloser
}

func (f *fileWrapper) Stat() (fs.FileInfo, error) { return f.fi, nil }

func (f *fileSystem) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).Stat(name)
	}
	h, err := f.hashFor(ctx)
	if err != nil {
		return nil, err
	}
	fi, _, err := f.stat(h, name)
	return fi, err
}

func (f *fileSystem) stat(h *plumbing.Hash, name string) (fs.FileInfo, *object.Tree, error) {
	c, err := f.bareRepo.CommitObject(*h)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.Tree()
	if err != nil {
		return nil, nil, err
	}
	te, err := t.FindEntry(name)
	if err != nil {
		// As part of the Stat contract, return os.ErrNotExist if the file doesn't exist
		return nil, nil, multierr.Combine(os.ErrNotExist, err)
	}
	fi, err := newFileInfo(te, t, c)
	return fi, t, err
}

func newFileInfo(te *object.TreeEntry, t *object.Tree, c *object.Commit) (*fileInfoWrapper, error) {
	sz, err := t.Size(te.Name)
	if err != nil {
		return nil, err
	}
	return &fileInfoWrapper{te, sz, c.Committer.When}, nil
}

type fileInfoWrapper struct {
	te         *object.TreeEntry
	sz         int64
	commitTime time.Time
}

func (i *fileInfoWrapper) Name() string               { return filepath.Base(i.te.Name) } // TODO: Needed?
func (i *fileInfoWrapper) Size() int64                { return i.sz }
func (i *fileInfoWrapper) ModTime() time.Time         { return i.commitTime }
func (i *fileInfoWrapper) IsDir() bool                { return i.Mode().IsDir() }
func (i *fileInfoWrapper) Sys() interface{}           { return nil }
func (i *fileInfoWrapper) Type() fs.FileMode          { return i.Mode() }
func (i *fileInfoWrapper) Info() (fs.FileInfo, error) { return i, nil }
func (i *fileInfoWrapper) Mode() fs.FileMode {
	fm, _ := i.te.Mode.ToOSFileMode()
	return fm
}

func (f *fileSystem) ReadDir(ctx context.Context, dirname string) ([]fs.DirEntry, error) {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).ReadDir(dirname)
	}
	h, err := f.hashFor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := f.bareRepo.CommitObject(*h)
	if err != nil {
		return nil, err
	}
	t, err := c.Tree()
	if err != nil {
		return nil, err
	}
	tw := object.NewTreeWalker(t, false, nil)
	infos := []fs.DirEntry{}
	for {
		_, te, err := tw.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		fi, err := newFileInfo(&te, t, c)
		if err != nil {
			return nil, err
		}
		infos = append(infos, fi)
	}
	return infos, nil
}

func (f *fileSystem) ReadFile(ctx context.Context, filename string) ([]byte, error) {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).ReadFile(filename)
	}
	return nil, ErrImmutableFilesystem // TODO
}

func (f *fileSystem) Checksum(ctx context.Context, filename string) (string, error) {
	if target, mutable := commit.GetMutableTarget(ctx); mutable {
		return f.mutableFSFor(ctx, target).Checksum(filename)
	}

	h, err := f.hashFor(ctx)
	if err != nil {
		return "", err
	}
	// Do a stat such that os.ErrNotExist is retuned if the file doesn't exist
	if _, _, err := f.stat(h, filename); err != nil {
		return "", err
	}
	return h.String(), nil
}
