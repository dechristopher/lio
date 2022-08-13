package store

import (
	"bytes"
	"context"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/dechristopher/lioctad/config"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
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

	// create data array
	data := make([]byte, info.Size)

	// read object into data array
	_, err = obj.Read(data)
	return data, err
}

// PutObject inserts an object into storage under the specified key
func (b Bucket) PutObject(key string, value []byte) error {
	reader := bytes.NewReader(value)
	_, err := C.PutObject(context.Background(), string(b), key,
		reader, int64(len(value)), minio.PutObjectOptions{
			UserMetadata: nil,
		})

	return err
}
