package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	URL   string
	Creds S3Credentials
}

func (s *S3Storage) parsePath() (bucket, key string, err error) {
	url := s.URL
	url = strings.TrimPrefix(url, "s3://")
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid S3 URL: %s", s.URL)
	}
	return parts[0], parts[1], nil
}

func (s *S3Storage) client(ctx context.Context) (*s3.Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(s.Creds.Region),
	}

	if s.Creds.AccessKeyID != "" && s.Creds.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				s.Creds.AccessKeyID,
				s.Creds.SecretAccessKey,
				"",
			),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if s.Creds.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = &s.Creds.Endpoint
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(cfg, s3Opts...), nil
}

func (s *S3Storage) Download(ctx context.Context, localPath string) error {
	bucket, key, err := s.parsePath()
	if err != nil {
		return err
	}

	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("S3 GetObject: %w", err)
	}
	defer result.Body.Close()

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := result.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write to local file: %w", writeErr)
			}
		}
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			}
			return fmt.Errorf("read from S3: %w", readErr)
		}
	}

	return nil
}

func (s *S3Storage) Upload(ctx context.Context, localPath string) error {
	bucket, key, err := s.parsePath()
	if err != nil {
		return err
	}

	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer file.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("S3 PutObject: %w", err)
	}

	return nil
}
