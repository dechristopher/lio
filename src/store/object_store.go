package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

var objectStoreAccessKeyID string
var objectStoreSecretAccessKey string
var objectStoreEndpoint string

type Bucket string

// PGNBucket is the name of the game PGN storage bucket
var PGNBucket Bucket

// C is the object storage client instance
var C *minio.Client

// Up brings the connection to the object store online
func Up() {
	objectStoreAccessKeyID = config.ReadSecretFallback("lio_obj_access")
	objectStoreSecretAccessKey = config.ReadSecretFallback("lio_obj_secret")
	objectStoreEndpoint = config.ReadSecretFallback("lio_obj_endpoint")

	PGNBucket = Bucket(config.ReadSecretFallback("lio_obj_bucket_pgn"))

	// a local dev boot without an object store configured is fine: warn and
	// skip. Game archival (the only consumer) degrades to a logged error per
	// finished game (Put/GetObject return an error while C is nil) instead of
	// refusing to boot the whole server.
	if env.IsLocal() && objectStoreEndpoint == "" {
		util.Info(str.CStor, "no object store configured; game archival disabled (local)")
		return
	}

	var err error

	// Initialize minio client object.
	C, err = minio.New(objectStoreEndpoint, &minio.Options{
		Creds: credentials.NewStaticV4(objectStoreAccessKeyID,
			objectStoreSecretAccessKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Fatalln(err)
	}

	// ensure credentials work properly
	buckets, err := C.ListBuckets(context.Background())
	if err != nil {
		log.Fatalln(str.CStor, str.EStoreInit, err.Error())
	}
	if len(buckets) == 0 {
		log.Fatalln(str.CStor, str.EStoreInit, "no buckets")
	}

	util.Debug(str.CStor, str.DStoreOk)
}

// GetObject pulls an object from storage as a byte array
func (b Bucket) GetObject(key string) ([]byte, error) {
	if C == nil {
		return nil, errors.New("store: no object store configured")
	}

	// pull object from store
	obj, err := C.GetObject(context.Background(), string(b),
		key, minio.GetObjectOptions{})

	if err != nil {
		return nil, err
	}

	// stat object for size of array to create
	info, err := obj.Stat()
	if err != nil {
		return nil, err
	}

	// read the object into a fully-sized array. io.ReadFull (not a single Read,
	// which may legally return fewer bytes) guarantees the whole object — the
	// archive backfill parses these bytes as complete PGNs.
	data := make([]byte, info.Size)
	_, err = io.ReadFull(obj, data)
	return data, err
}

// Configured reports whether an object store connection is available.
func Configured() bool {
	return C != nil
}

// ListKeys returns every object key in the bucket (recursive). Used by the
// archive backfill to enumerate stored PGNs; keys are small strings, so the
// streamed listing is collected into a slice.
func (b Bucket) ListKeys(ctx context.Context) ([]string, error) {
	if C == nil {
		return nil, errors.New("store: no object store configured")
	}

	var keys []string
	for obj := range C.ListObjects(ctx, string(b), minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// PutObject inserts an object into storage under the specified key
func (b Bucket) PutObject(key string, value []byte) error {
	if C == nil {
		return errors.New("store: no object store configured")
	}

	reader := bytes.NewReader(value)
	_, err := C.PutObject(context.Background(), string(b), key,
		reader, int64(len(value)), minio.PutObjectOptions{
			UserMetadata: nil,
		})

	return err
}
