package reconcile

import (
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/client"
	"github.com/weaveworks/libgitops/pkg/storage/cache"
	"github.com/weaveworks/libgitops/pkg/storage/manifest"
)

var c *client.Client

func ReconcileManifests(s *manifest.ManifestStorage) {
	startMetricsThread()

	// Wrap the Manifest Storage with a cache for better performance, and create a client
	c = client.NewClient(cache.NewCache(s))

	// These updates are coming from the SyncStorage
	for upd := range s.GetUpdateStream() {
		// Just log here
		log.Infof("Got update in reconciliation loop: %v", upd)
	}
}
