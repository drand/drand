package boltdb_test

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestStorageFormatFillPercentage(t *testing.T) {
	tempDir := t.TempDir()

	db, err := bbolt.Open(path.Join(tempDir, "formatfill.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	db.MaxBatchSize = 100

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	var values [][]byte
	for i := 0; i < db.MaxBatchSize*100; i++ {
		values = append(values, []byte(fmt.Sprintf("index %d value %d", i, rnd.Int())))
	}

	bucketName := "my-test"

	fillPercentages := []float64{0.5, 1.0, 0.75, 0.5}

	for idx, fillPercentage := range fillPercentages {
		err = storeValues(t, db, bucketName, idx*len(values), fillPercentage, values)
		require.NoError(t, err)
	}

	for idx := range fillPercentages {
		err = testStoredValues(t, db, bucketName, idx*len(values), values)
		require.NoError(t, err)
	}
}

func storeValues(t *testing.T, db *bbolt.DB, bucketName string, offset int, fillPercentage float64, values [][]byte) error {
	return db.Batch(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		require.NoError(t, err)

		bucket.FillPercent = fillPercentage

		newKey := make([]byte, 8)
		for idx, val := range values {
			binary.BigEndian.PutUint64(newKey, uint64(idx+offset))
			require.NoError(t, bucket.Put(newKey, val))
		}

		return nil
	})
}

func testStoredValues(t *testing.T, db *bbolt.DB, bucketName string, offset int, values [][]byte) error {
	return db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))

		newKey := make([]byte, 8)
		for idx := 0; idx < len(values); idx++ {
			binary.BigEndian.PutUint64(newKey, uint64(idx+offset))
			value := bucket.Get(newKey)
			require.Equal(t, string(value), string(values[idx]))
		}

		return nil
	})
}
