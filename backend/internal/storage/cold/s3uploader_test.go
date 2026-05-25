// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: backend/internal/storage/cold/s3uploader_test.go
// Purpose: Unit tests for the S3 uploader, mocking the AWS SDK v2 client.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package cold

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockS3Client is a mock of the S3 API.
type MockS3Client struct {
	mock.Mock
}

func (m *MockS3Client) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*s3.CreateBucketOutput), args.Error(1)
}

func (m *MockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*s3.PutObjectOutput), args.Error(1)
}

func TestS3Uploader_UploadFile(t *testing.T) {
	mockClient := new(MockS3Client)
	cfg := &Config{
		Bucket: "test-bucket",
		Prefix: "test-prefix/",
	}
	uploader := &S3Uploader{
		client: mockClient,
		cfg:    cfg,
	}

	ctx := context.Background()
	localPath := "test_upload.parquet"
	err := os.WriteFile(localPath, []byte("test data"), 0644)
	assert.NoError(t, err)
	defer os.Remove(localPath)

	objectKey := "metrics/test.parquet"

	mockClient.On("PutObject", ctx, mock.MatchedBy(func(input *s3.PutObjectInput) bool {
		return *input.Bucket == "test-bucket" && *input.Key == objectKey
	})).Return(&s3.PutObjectOutput{}, nil)

	err = uploader.UploadFile(ctx, localPath, objectKey)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestS3Uploader_ObjectKey(t *testing.T) {
	cfg := &Config{
		Prefix: "metrics/",
	}
	uploader := &S3Uploader{cfg: cfg}

	key := uploader.ObjectKey("sqlserver", "cpu_history", "server1", "2025", "05", "22", 1)
	expected := "metrics/engine=sqlserver/table=cpu_history/server_id=server1/year=2025/month=05/day=22/part-000001.parquet"
	assert.Equal(t, expected, key)
}
