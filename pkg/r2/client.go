package r2

import (
	"context"
	"fmt"
	"io"

	"girls-rating-api/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client Cloudflare R2 客户端（S3 兼容）
type Client struct {
	s3Client *s3.Client
	bucket   string
}

// New 创建 R2 客户端
func New(cfg config.R2Config) (*Client, error) {
	if cfg.AccountID == "" || cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("r2 config is incomplete: account_id, access_key, secret_key, bucket are required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint())
	})

	return &Client{
		s3Client: s3Client,
		bucket:   cfg.Bucket,
	}, nil
}

// Upload 上传文件到 R2
// key: 对象 key，如 "images/2024/01/01/1234567890_abc.jpg"
// contentType: MIME 类型，如 "image/jpeg"
// body: 文件内容
// 返回对象 key（相对路径），由调用方决定是否拼接公开 URL
func (c *Client) Upload(ctx context.Context, key string, contentType string, body io.Reader) (string, error) {
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to r2: %w", err)
	}

	return key, nil
}
