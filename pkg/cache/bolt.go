package cache

import (
	"time"

	"github.com/boltdb/bolt"
	"k8s.io/klog"
)

var db *bolt.DB = nil

func Open(path string) {
	var err error
	db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		klog.Fatalf("Failed opening cache at %s: %v", path, err)
	}
}

func Close() {
	if db != nil {
		db.Close()
	}
}

func Get(name string, revision string) ([]byte, error) {
	if db == nil || revision == "" {
		return nil, nil
	}

	var res []byte
	var resErr error
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b == nil {
			res, resErr = nil, nil
			return nil
		}
		v := b.Get([]byte("revision"))
		if revision != "" && string(v) != revision {
			res, resErr = nil, nil
			return nil
		}
		v = b.Get([]byte("data"))
		res, resErr = make([]byte, len(v)), nil
		copy(res, v)
		klog.Infof("Cache hit for %q revision %q", name, revision)
		return nil
	})
	return res, resErr
}

func Set(name string, revision string, data []byte) {
	if db == nil || revision == "" {
		return
	}

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b == nil {
			b, _ = tx.CreateBucket([]byte(name))
		}

		klog.Infof("Caching %q at revision %q", name, revision)
		err := b.Put([]byte("revision"), []byte(revision))
		if err != nil {
			return err
		}
		err = b.Put([]byte("data"), data)
		return err
	})
}
