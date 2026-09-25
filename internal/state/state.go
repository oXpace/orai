// Package state holds private local state: atomic JSON, per-role locks and
// superseded-session history. Directories are 0700 and files 0600.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// PrivateDirs creates every missing directory as 0700. os.MkdirAll would apply the
// mode only after umask, and callers rely on the exact permissions.
func PrivateDirs(dir string) error {
	var missing []string
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := os.Chmod(missing[i], 0o700); err != nil {
			return err
		}
	}
	return nil
}

// AtomicWrite writes data to a temporary file in the same directory, syncs it and
// renames it over path, so readers see either the old or the new content.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := PrivateDirs(filepath.Dir(path)); err != nil {
		return err
	}
	return WriteReplace(path, data, mode)
}

// WriteReplace is AtomicWrite for a directory that already exists (shared project
// folders keep their own permissions).
func WriteReplace(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".orai-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func JSONBytes(value any) []byte {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err) // only plain maps/structs are written
	}
	return append(data, '\n')
}

func WriteJSON(path string, value any) error { return AtomicWrite(path, JSONBytes(value), 0o600) }

// ReadJSON decodes path into a map; a missing file returns (nil, nil).
func ReadJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

// RoleFiles is the layout of one role inside a project's role-state directory.
type RoleFiles struct {
	Dir, Role string
}

func (f RoleFiles) State() string         { return filepath.Join(f.Dir, f.Role+".json") }
func (f RoleFiles) Lock() string          { return filepath.Join(f.Dir, f.Role+".lock") }
func (f RoleFiles) CodexDelivery() string { return filepath.Join(f.Dir, f.Role+".delivery.json") }
func (f RoleFiles) ClaudeChannel() string { return filepath.Join(f.Dir, f.Role+".channel.json") }

// Delivery is the provider-specific notifier state; readiness is judged per run nonce.
func (f RoleFiles) Delivery(provider string) string {
	if provider == "claude" {
		return f.ClaudeChannel()
	}
	return f.CodexDelivery()
}

// Archive keeps a superseded state (for example before --fresh) in history/.
func (f RoleFiles) Archive(value map[string]any) error {
	name := f.Role + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".json"
	return WriteJSON(filepath.Join(filepath.Dir(f.Dir), "history", name), value)
}

var ErrRunning = errors.New("already running")

// RoleLock takes the role's exclusive lock. The returned file *is* the lock; pass it
// to the child so the lock lives as long as the provider does.
func RoleLock(f RoleFiles) (*os.File, error) {
	if err := PrivateDirs(f.Dir); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(f.Lock(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s is %w; use its existing terminal", f.Role, ErrRunning)
		}
		return nil, err
	}
	return file, nil
}

// Locked reports whether another process holds the role lock.
func Locked(f RoleFiles) bool {
	file, err := os.Open(f.Lock())
	if err != nil {
		return false
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.Is(err, syscall.EWOULDBLOCK)
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false
}
