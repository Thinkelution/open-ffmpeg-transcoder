package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

// Downloader fetches a remote file to a local path.
type Downloader interface {
	Download(ctx context.Context, localPath string) error
}

// Uploader pushes a local file to a remote destination.
type Uploader interface {
	Upload(ctx context.Context, localPath string) error
}

// S3Credentials holds per-job S3 override credentials.
type S3Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
}

// FTPCredentials holds FTP server credentials.
type FTPCredentials struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewDownloader creates the appropriate downloader based on storage config.
func NewDownloader(sc database.StorageConfig, cfg *config.Config) (Downloader, error) {
	switch sc.Type {
	case "http", "https":
		return &HTTPDownloader{URL: sc.URL}, nil
	case "s3":
		creds := resolveS3Creds(sc.Credentials, cfg)
		return &S3Storage{URL: sc.URL, Creds: creds}, nil
	case "ftp":
		creds, err := parseFTPCreds(sc.Credentials)
		if err != nil {
			return nil, err
		}
		return &FTPStorage{URL: sc.URL, Creds: creds}, nil
	case "local":
		return &LocalStorage{Path: sc.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported input type: %s", sc.Type)
	}
}

// NewUploader creates the appropriate uploader based on storage config.
func NewUploader(sc database.StorageConfig, cfg *config.Config) (Uploader, error) {
	switch sc.Type {
	case "s3":
		creds := resolveS3Creds(sc.Credentials, cfg)
		return &S3Storage{URL: sc.URL, Creds: creds}, nil
	case "ftp":
		creds, err := parseFTPCreds(sc.Credentials)
		if err != nil {
			return nil, err
		}
		return &FTPStorage{URL: sc.URL, Creds: creds}, nil
	case "local":
		return &LocalStorage{Path: sc.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported output type: %s", sc.Type)
	}
}

func resolveS3Creds(raw json.RawMessage, cfg *config.Config) S3Credentials {
	var creds S3Credentials
	if raw != nil {
		json.Unmarshal(raw, &creds)
	}
	// Fall back to global config
	if creds.AccessKeyID == "" {
		creds.AccessKeyID = cfg.S3AccessKey
	}
	if creds.SecretAccessKey == "" {
		creds.SecretAccessKey = cfg.S3SecretKey
	}
	if creds.Region == "" {
		creds.Region = cfg.S3Region
	}
	if creds.Endpoint == "" {
		creds.Endpoint = cfg.S3Endpoint
	}
	return creds
}

func parseFTPCreds(raw json.RawMessage) (FTPCredentials, error) {
	var creds FTPCredentials
	if raw != nil {
		if err := json.Unmarshal(raw, &creds); err != nil {
			return creds, fmt.Errorf("parse FTP credentials: %w", err)
		}
	}
	if creds.Port == 0 {
		creds.Port = 21
	}
	return creds, nil
}
