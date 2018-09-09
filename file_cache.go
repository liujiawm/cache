// Package file is a simple local file system cache implement.
package cache

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// FileCache definition.
type FileCache struct {
	// caches in memory
	MemoryCache
	// cache directory path
	cacheDir string
	// cache file prefix
	prefix string
	// security key for generate cache file name.
	securityKey string
}

// New a FileCache instance
func NewFileCache(dir string, pfxAndKey ...string) *FileCache {
	if dir == "" { // empty, use system tmp dir
		dir = os.TempDir()
	}

	c := &FileCache{
		cacheDir: dir,
		// init a memory cache.
		MemoryCache: MemoryCache{caches: make(map[string]*CacheItem)},
	}

	if ln := len(pfxAndKey); ln > 0 {
		c.prefix = pfxAndKey[0]

		if ln > 1 {
			c.securityKey = pfxAndKey[1]
		}
	}

	return c
}

// Has cache key.
// TODO decode value, and check expire time
func (c *FileCache) Has(key string) bool {
	if c.MemoryCache.Has(key) {
		return true
	}

	path := c.GetFilename(key)
	return fileExists(path)
}

func (c *FileCache) Get(key string) interface{} {
	// read cache from memory
	if val := c.MemoryCache.Get(key); val != nil {
		return val
	}

	c.lock.RLock()
	defer c.lock.RUnlock()

	// read cache from file
	bs, err := ioutil.ReadFile(c.GetFilename(key))
	if err != nil {
		c.lastErr = err
		return nil
	}

	item := &CacheItem{}
	if err = Unmarshal(bs, item); err != nil {
		c.lastErr = err
		return nil
	}

	// check expire time
	if item.Exp == 0 || item.Exp > time.Now().Unix() {
		c.caches[key] = item // save to memory.
		return item.Val
	}

	// has been expired. delete it.
	c.Del(key)
	return nil
}

func (c *FileCache) Set(key string, val interface{}, ttl time.Duration) (err error) {
	if err = c.MemoryCache.Set(key, val, ttl); err != nil {
		c.lastErr = err
		return
	}

	// cache item data to file
	bs, err := Marshal(c.caches[key])
	if err != nil {
		c.lastErr = err
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	file := c.GetFilename(key)
	dir := filepath.Dir(file)
	if err = os.MkdirAll(dir, 0755); err != nil {
		c.lastErr = err
		return
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(bs); err != nil {
		return err
	}

	return
}

// Del value by key
func (c *FileCache) Del(key string) error {
	c.MemoryCache.Del(key)

	file := c.GetFilename(key)
	if fileExists(file) {
		return os.Remove(file)
	}

	return nil
}

// GetMulti values by multi key
func (c *FileCache) GetMulti(keys []string) []interface{} {
	var values []interface{}
	for _, key := range keys {
		values = append(values, c.Get(key))
	}

	return values
}

// SetMulti values by multi key
func (c *FileCache) SetMulti(values map[string]interface{}, ttl time.Duration) (err error) {
	for key, val := range values {
		if err = c.Set(key, val, ttl); err != nil {
			return
		}
	}

	return
}

// DelMulti values by multi key
func (c *FileCache) DelMulti(keys []string) error {
	for _, key := range keys {
		c.Del(key)
	}
	return nil
}

// Clear caches and files
func (c *FileCache) Clear() error {
	for key := range c.caches {
		file := c.GetFilename(key)

		if fileExists(file) {
			err := os.Remove(file)
			if err != nil {
				return err
			}
		}
	}

	c.caches = nil
	// clear cache files
	return os.RemoveAll(c.cacheDir)
}

// GetFilename cache file name build
func (c *FileCache) GetFilename(key string) string {
	h := md5.New()
	if c.securityKey != "" {
		h.Write([]byte(c.securityKey + key))
	} else {
		h.Write([]byte(key))
	}

	str := hex.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%s/%s/%s.data", c.cacheDir, str[0:6], c.prefix+str)
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}