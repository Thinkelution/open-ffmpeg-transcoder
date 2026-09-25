package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local path: %w", err)
	}
	if stat.IsDir() {
		return s.uploadDir(ctx, client, bucket, key, localPath)
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

func (s *S3Storage) uploadDir(ctx context.Context, client *s3.Client, bucket, prefix, localDir string) error {
	prefix = strings.TrimSuffix(prefix, "/")
	return filepath.WalkDir(localDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		objectKey := strings.TrimPrefix(prefix+"/"+filepath.ToSlash(rel), "/")

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open local file: %w", err)
		}
		defer file.Close()

		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      &bucket,
			Key:         &objectKey,
			Body:        file,
			ContentType: contentTypeForKey(objectKey),
		})
		if err != nil {
			return fmt.Errorf("S3 PutObject %s: %w", objectKey, err)
		}
		return nil
	})
}

func contentTypeForKey(key string) *string {
	contentType := ""
	switch strings.ToLower(filepath.Ext(key)) {
	case ".m3u8":
		contentType = "application/vnd.apple.mpegurl"
	case ".ts":
		contentType = "video/mp2t"
	case ".mp4":
		contentType = "video/mp4"
	}
	if contentType == "" {
		return nil
	}
	return &contentType
}

// ListObjects returns object keys under the provided prefix.
func ListObjects(ctx context.Context, creds S3Credentials, bucket, prefix string) ([]types.Object, error) {
	store := &S3Storage{Creds: creds}
	client, err := store.client(ctx)
	if err != nil {
		return nil, err
	}

	var objects []types.Object
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("S3 ListObjectsV2: %w", err)
		}
		objects = append(objects, page.Contents...)
	}
	return objects, nil
}

func CopyDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		dst := filepath.Join(dstDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		src, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open source file: %w", err)
		}
		defer src.Close()
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		out, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer out.Close()
		if _, err := io.Copy(out, src); err != nil {
			return fmt.Errorf("copy file: %w", err)
		}
		return nil
	})
}
