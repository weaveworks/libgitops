package unstructured

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// ErrOnlySingleFrameSupported tells that only single frame-files are supported so far for the unstructured Storage.
var ErrOnlySingleFrameSupported = errors.New("file contains multiple Objects; for now only single-frame files are supported")

func NewGeneric(storage filesystem.Storage, recognizer ObjectRecognizer, pathExcluder filesystem.PathExcluder) (Storage, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is mandatory")
	}
	if recognizer == nil {
		return nil, fmt.Errorf("recognizer is mandatory")
	}
	fileFinder, ok := storage.FileFinder().(FileFinder)
	if !ok {
		return nil, errors.New("the given filesystem.Storage must use a unstructured.FileFinder")
	}
	return &Generic{
		Storage:      storage,
		recognizer:   recognizer,
		fileFinder:   fileFinder,
		pathExcluder: pathExcluder,
	}, nil
}

type Generic struct {
	filesystem.Storage
	recognizer   ObjectRecognizer
	fileFinder   FileFinder
	pathExcluder filesystem.PathExcluder
}

// Sync synchronizes the current state of the filesystem with the
// cached mappings in the underlying unstructured.FileFinder.
func (s *Generic) Sync(ctx context.Context) ([]ChecksumPathID, error) {
	fileFinder := s.UnstructuredFileFinder()

	// List all valid files in the fs
	files, err := filesystem.ListValidFilesInFilesystem(
		ctx,
		fileFinder.Filesystem(),
		fileFinder.ContentTyper(),
		s.PathExcluder(),
	)
	if err != nil {
		return nil, err
	}

	// Send SYNC events for all files (and fill the mappings
	// of the unstructured.FileFinder) before starting to monitor changes
	updatedFiles := make([]ChecksumPathID, 0, len(files))
	for _, filePath := range files {
		// Get the current checksum of the file
		currentChecksum, err := fileFinder.Filesystem().Checksum(ctx, filePath)
		if err != nil {
			logrus.Errorf("Could not get checksum for file %q: %v", filePath, err)
			continue
		}

		// If the given file already is tracked; i.e. has a mapping with a
		// non-empty checksum, and the current checksum matches, we do not
		// need to do anything.
		if id, err := SingleObjectAt(ctx, fileFinder, filePath); err == nil {
			if cp, ok := fileFinder.GetMapping(ctx, id); ok && len(cp.Checksum) != 0 {
				if cp.Checksum == currentChecksum {
					logrus.Tracef("Checksum for file %q is up-to-date: %q, skipping...", filePath, cp.Checksum)
					continue
				}
			}
		}

		// Read and recognize the file
		id, err := ReadAndRecognizeFile(
			ctx,
			fileFinder.Filesystem(),
			fileFinder.ContentTyper(),
			s.recognizer,
			filePath,
		)
		if err != nil {
			logrus.Warn(err)
			continue
		}

		// Add a mapping between this object and path
		cp := ChecksumPath{
			Checksum: currentChecksum,
			Path:     filePath,
		}
		fileFinder.SetMapping(ctx, id, cp)
		// Add to the slice which we'll return
		updatedFiles = append(updatedFiles, ChecksumPathID{
			ChecksumPath: cp,
			ID:           id,
		})
	}
	return updatedFiles, nil
}

// ObjectRecognizer returns the underlying ObjectRecognizer used.
func (s *Generic) ObjectRecognizer() ObjectRecognizer {
	return s.recognizer
}

// PathExcluder specifies what paths to not sync
func (s *Generic) PathExcluder() filesystem.PathExcluder {
	return s.pathExcluder
}

// UnstructuredFileFinder returns the underlying unstructured.FileFinder used.
func (s *Generic) UnstructuredFileFinder() FileFinder {
	return s.fileFinder
}

// ReadAndRecognizeFile reads the given file and its content type; and then recognizes it.
// It only supports one ObjectID per file at the moment.
func ReadAndRecognizeFile(
	ctx context.Context,
	fs filesystem.Filesystem,
	contentTyper filesystem.ContentTyper,
	recognizer ObjectRecognizer,
	filePath string,
) (core.ObjectID, error) {
	// If the file is not known to the FileFinder yet, or if the checksum
	// was empty, read the file, and recognize it.
	content, err := fs.ReadFile(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("Could not read file %q: %v", filePath, err)
	}
	// Get the content type for this file so that we can read it properly
	ct, err := contentTyper.ContentTypeForPath(ctx, fs, filePath)
	if err != nil {
		return nil, fmt.Errorf("Could not get content type for file %q: %v", filePath, err)
	}
	// TODO: In the future this NewFrameReader should come from an interface, not
	// directly from the hard-coded serializer package.
	fr := serializer.NewFrameReader(ct, serializer.FromBytes(content))
	// Recognize all IDs in the file
	ids, err := recognizer.RecognizeObjectIDs(filePath, fr)
	if err != nil {
		return nil, fmt.Errorf("Could not recognize object IDs in %q: %v", filePath, err)
	}
	// For now; we only support single-frame files
	// TODO: Change this.
	if ids.Len() != 1 {
		return nil, fmt.Errorf("%w: %q", ErrOnlySingleFrameSupported, filePath)
	}
	// Return that one ID
	return ids.List()[0], nil
}

func SingleObjectAt(ctx context.Context, fileFinder filesystem.FileFinder, filePath string) (core.UnversionedObjectID, error) {
	idSet, err := fileFinder.ObjectsAt(ctx, filePath)
	if err != nil {
		return nil, err
	}
	// For now; we only support single-frame files
	// TODO: Change this.
	if idSet.Len() != 1 {
		return nil, fmt.Errorf("%w: %q", ErrOnlySingleFrameSupported, filePath)
	}
	// Return that one ID
	return idSet.List()[0], nil
}
