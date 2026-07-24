// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/s3uploader.go
// Purpose: S3-compatible object storage client for uploading Parquet files to MinIO or AWS S3.
//          Uses multipart upload via the AWS S3 Transfer Manager for large-file robustness.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	// multipartPartSize is the target part size for multipart uploads (5 MB).
	// The AWS minimum is 5 MB; this keeps part counts low for typical Parquet files.
	multipartPartSize = 5 * 1024 * 1024
	// multipartConcurrency controls how many S3 parts are uploaded concurrently.
	multipartConcurrency = 4
)

// S3BucketAPI is the minimal interface used by EnsureBucket.
type S3BucketAPI interface {
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

// s3ManagerAPI abstracts manager.Uploader for testability.
type s3ManagerAPI interface {
	Upload(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

// S3Uploader wraps the AWS S3 client and transfer manager for cold storage uploads.
type S3Uploader struct {
	bucket  S3BucketAPI
	manager s3ManagerAPI
	cfg     *Config
}

// NewS3Uploader creates an S3 client configured for either MinIO or AWS S3.
func NewS3Uploader(ctx context.Context, cfg *Config) (*S3Uploader, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("cold/s3: failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle
	})

	mgr := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = multipartPartSize
		u.Concurrency = multipartConcurrency
	})

	return &S3Uploader{bucket: client, manager: mgr, cfg: cfg}, nil
}

// EnsureBucket creates the bucket if it does not exist. Idempotent.
// BucketAlreadyOwnedByYou and BucketAlreadyExists are treated as success.
func (u *S3Uploader) EnsureBucket(ctx context.Context) error {
	_, err := u.bucket.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(u.cfg.Bucket),
	})
	if err != nil {
		var owned *types.BucketAlreadyOwnedByYou
		var exists *types.BucketAlreadyExists
		if errors.As(err, &owned) || errors.As(err, &exists) {
			return nil
		}
		return fmt.Errorf("cold/s3: ensure bucket %q: %w", u.cfg.Bucket, err)
	}
	return nil
}

// UploadFile uploads a local file to S3 at the given object key using multipart upload.
// Files under 5 MB are sent in a single PUT; larger files are chunked automatically.
func (u *S3Uploader) UploadFile(ctx context.Context, localPath, objectKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cold/s3: open local file %q: %w", localPath, err)
	}
	defer f.Close()

	_, err = u.manager.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.cfg.Bucket),
		Key:         aws.String(objectKey),
		Body:        f,
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("cold/s3: upload %q: %w", objectKey, err)
	}
	return nil
}

// ObjectKey builds the Hive-style partition path for a given export.
func (u *S3Uploader) ObjectKey(engine, table, serverID, year, month, day string, part int) string {
	return fmt.Sprintf(
		"%sengine=%s/table=%s/server_id=%s/year=%s/month=%s/day=%s/part-%06d.parquet",
		u.cfg.Prefix, engine, table, serverID, year, month, day, part,
	)
}
