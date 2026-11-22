package shelf

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oXpace/orai/internal/project"
)

// autoSyncDebounce is how long the documents must stay quiet before a sync: a burst of
// saves becomes one sync. Tests shorten it.
var autoSyncDebounce = 3 * time.Second

// autoSyncRun is the sync AutoSync performs; tests replace it so no qmd runs.
var autoSyncRun = func(s *Settings, out io.Writer) error { return lifecycle(s, "sync", out) }

// AutoSync keeps the shelf index in step with the documents while a role session runs,
// so nobody has to remember `orai shelf sync` after editing a document. It watches the
// declared collection folders and, once they have been quiet for a moment, runs the
// same sync as `orai shelf sync` (which also starts the server if it is down). A sync
// left over from while no session ran is caught up at start.
//
// It never builds a shelf that was not built (`orai shelf init` stays a user step),
// writes nothing to the terminal (the provider owns it; output goes to
// .orai/shelf/autosync.log), and gives way when another orai shelf command holds the
// lock, trying again later. Several roles may run it at once: the lock serializes them
// and the sync marker lets a later one skip what an earlier one already did.
func AutoSync(ctx context.Context, p *project.Project) {
	if p.Config.Shelf == nil {
		return
	}
	if _, err := lookPath("qmd"); err != nil {
		return
	}
	s := NewSettings(p)
	if !exists(s.DB) || !exists(s.ConfigFile) {
		return
	}
	log, err := os.OpenFile(filepath.Join(s.Directory, "autosync.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer log.Close()
	s.Log = log
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(log, "%s autosync off: %v\n", stamp(), err)
		return
	}
	defer watcher.Close()
	for _, name := range s.collectionNames() {
		watchTree(watcher, s.Collections[name], nil)
	}

	var changed time.Time // last relevant change not yet covered by a sync
	timer := time.NewTimer(autoSyncDebounce)
	if len(s.staleDocuments()) == 0 {
		if !timer.Stop() {
			<-timer.C
		}
	} else {
		changed = time.Now()
	}
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(log, "%s watch error: %v\n", stamp(), err)
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			relevant := s.covers(event.Name)
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					// Files written into a new folder before it was watched raise no
					// event of their own, so look inside once it is.
					relevant = watchTree(watcher, event.Name, s.covers) || relevant
				}
			}
			if !relevant {
				continue
			}
			changed = time.Now()
			timer.Reset(autoSyncDebounce)
		case <-timer.C:
			if changed.IsZero() {
				continue
			}
			if synced, err := os.Stat(s.SyncedFile()); err == nil && synced.ModTime().After(changed) {
				changed = time.Time{} // another role's sync already covered it
				continue
			}
			fmt.Fprintf(log, "%s sync\n", stamp())
			if err := autoSyncRun(s, log); err != nil {
				fmt.Fprintf(log, "%s sync failed (will retry on the next change): %v\n", stamp(), err)
				if strings.Contains(err.Error(), "Another orai shelf command") {
					timer.Reset(autoSyncDebounce) // someone else is syncing: look again soon
				}
				continue
			}
			changed = time.Time{}
		}
	}
}

// watchTree adds root and its folders (hidden ones skipped) to the watcher, and
// reports whether it holds a file match accepts (match may be nil).
func watchTree(w *fsnotify.Watcher, root string, match func(string) bool) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			found = found || (match != nil && match(path))
			return nil
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		_ = w.Add(path)
		return nil
	})
	return found
}

// covers reports whether path is (or was, for a removal) a document of a declared
// collection.
func (s *Settings) covers(path string) bool {
	for _, name := range s.collectionNames() {
		rel, err := filepath.Rel(s.Collections[name], path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if MatchPattern(s.Patterns[name], filepath.ToSlash(rel)) {
			return true
		}
	}
	return false
}

func stamp() string { return time.Now().Format(time.RFC3339) }
