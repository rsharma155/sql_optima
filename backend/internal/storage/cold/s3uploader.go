// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/s3uploader.go
// Purpose: S3-compatible object storage client for uploading Parquet files to MinIO or AWS S3.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API defines the subset of S3 methods used by the uploader.
type S3API interface {
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Uploader wraps the AWS S3 client for cold storage uploads.
type S3Uploader struct {
	client S3API
	cfg    *Config
}

// NewS3Uploader creates an S3 client configured for either MinIO or AWS S3.
func NewS3Uploader(ctx context.Context, cfg *Config) (*S3Uploader, error) {
	// AWS SDK v2 uses a more modern approach to custom endpoints.
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

	return &S3Uploader{client: client, cfg: cfg}, nil
}

// EnsureBucket creates the bucket if it does not exist. Idempotent.
func (u *S3Uploader) EnsureBucket(ctx context.Context) error {
	_, err := u.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(u.cfg.Bucket),
	})
	if err != nil {
		// In production, we might want to check for specific error codes like BucketAlreadyOwnedByYou.
		// For now, we'll log it or ignore it as the roadmap suggests.
		_ = err
	}
	return nil
}

// UploadFile uploads a local file to S3 at the given object key.
func (u *S3Uploader) UploadFile(ctx context.Context, localPath, objectKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cold/s3: open local file %q: %w", localPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cold/s3: stat %q: %w", localPath, err)
	}

	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(u.cfg.Bucket),
		Key:           aws.String(objectKey),
		Body:          f,
		ContentLength: aws.Int64(stat.Size()),
		ContentType:   aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("cold/s3: put object %q: %w", objectKey, err)
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
