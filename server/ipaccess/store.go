// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ipaccess

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fatedier/frp/pkg/util/log"
)

const (
	// flushInterval is how often the dirty access records are written to disk.
	// Records only live in memory between two flushes, so a crash may lose the
	// last minute of them; rules are written synchronously and are never lost.
	flushInterval = 60 * time.Second
	// cleanupInterval is how often the expired records are removed.
	cleanupInterval = 5 * time.Minute
)

// load reads the store file. A missing file means an empty store.
func (c *Controller) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read ip access store %s error: %v", c.path, err)
	}

	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse ip access store %s error: %v", c.path, err)
	}

	if s.GlobalMode != "" {
		if mode, err := normalizeMode(s.GlobalMode); err == nil {
			c.globalMode = mode
		}
	}
	for name, mode := range s.ProxyModes {
		if m, err := normalizeMode(mode); err == nil {
			c.proxyModes[name] = m
		}
	}
	c.rules = s.Rules
	c.index = buildRuleIndex(c.rules)

	var maxID uint64
	for i := range s.Records {
		r := s.Records[i]
		c.records[recordKey(r.ProxyName, r.IP)] = &r
		maxID = max(maxID, r.ID)
	}
	for _, r := range c.rules {
		maxID = max(maxID, r.ID)
	}
	c.nextID.Store(max(maxID, s.NextID))

	log.Infof("ip access control: loaded %d rules and %d access records from %s",
		len(c.rules), len(c.records), c.path)
	return nil
}

// save writes the whole store to disk through a temporary file, so that a crash
// during the write never leaves a truncated store behind.
func (c *Controller) save() error {
	c.recordMu.Lock()
	c.dirty = false
	c.recordMu.Unlock()

	data, err := json.Marshal(c.snapshot())
	if err != nil {
		return fmt.Errorf("encode ip access store error: %v", err)
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s error: %v", dir, err)
	}
	tmpPath := c.path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create ip access store %s error: %v", tmpPath, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write ip access store %s error: %v", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync ip access store %s error: %v", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close ip access store %s error: %v", tmpPath, err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace ip access store %s error: %v", c.path, err)
	}
	return nil
}

func (c *Controller) snapshot() *snapshot {
	c.ruleMu.RLock()
	s := &snapshot{
		Version:    snapshotVersion,
		NextID:     c.nextID.Load(),
		GlobalMode: c.globalMode,
		ProxyModes: maps.Clone(c.proxyModes),
		Rules:      slices.Clone(c.rules),
	}
	c.ruleMu.RUnlock()

	c.recordMu.Lock()
	s.Records = make([]Record, 0, len(c.records))
	for _, r := range c.records {
		s.Records = append(s.Records, *r)
	}
	c.recordMu.Unlock()
	return s
}

func (c *Controller) isDirty() bool {
	c.recordMu.Lock()
	defer c.recordMu.Unlock()
	return c.dirty
}

func (c *Controller) markDirty() {
	c.recordMu.Lock()
	c.dirty = true
	c.recordMu.Unlock()
}

func (c *Controller) flushLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			if !c.isDirty() {
				continue
			}
			if err := c.save(); err != nil {
				log.Warnf("ip access control: %v", err)
				c.markDirty()
			}
		}
	}
}

func (c *Controller) cleanupLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			if removed := c.Prune(); removed > 0 {
				log.Infof("ip access control: removed %d expired access records", removed)
			}
		}
	}
}

// Close stops the background workers and flushes the pending records.
func (c *Controller) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.stop)
		c.wg.Wait()
		if c.isDirty() {
			err = c.save()
		}
	})
	return err
}
