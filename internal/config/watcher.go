// Package config provides config hot-reload via SIGHUP and file watching.
package config

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// ReloadFunc reloads configuration from the given path.
type ReloadFunc func(configPath string) error

// SetupSIGHUPHandler reloads config whenever the process receives SIGHUP.
func SetupSIGHUPHandler(configPath string, reload ReloadFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for range ch {
			log.Info("received SIGHUP, reloading configuration")
			if err := reload(configPath); err != nil {
				log.WithError(err).Error("config reload failed")
			}
		}
	}()
}

// WatchConfigFile watches the config file's directory and reloads on write or
// create events (watching the directory handles editors that rename-on-save).
// The returned watcher must be closed by the caller during shutdown.
func WatchConfigFile(configPath string, reload ReloadFunc) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(configPath)
	name := filepath.Base(configPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	go func() {
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(ev.Name) != name {
					continue
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				log.WithField("path", configPath).Info("config file changed, reloading")
				if err := reload(configPath); err != nil {
					log.WithError(err).Error("config reload failed")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.WithError(err).Error("config watcher error")
			}
		}
	}()
	return watcher, nil
}
