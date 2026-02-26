package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/jlaffaye/ftp"
)

type FTPStorage struct {
	URL   string
	Creds FTPCredentials
}

func (f *FTPStorage) parsePath() (string, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return "", fmt.Errorf("parse FTP URL: %w", err)
	}
	return u.Path, nil
}

func (f *FTPStorage) connect(_ context.Context) (*ftp.ServerConn, error) {
	addr := fmt.Sprintf("%s:%d", f.Creds.Host, f.Creds.Port)

	// Also try to parse host from URL if creds.Host is empty
	if f.Creds.Host == "" {
		u, err := url.Parse(f.URL)
		if err != nil {
			return nil, fmt.Errorf("parse FTP URL for host: %w", err)
		}
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = "21"
		}
		addr = fmt.Sprintf("%s:%s", host, port)

		if u.User != nil {
			f.Creds.Username = u.User.Username()
			f.Creds.Password, _ = u.User.Password()
		}
	}

	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("FTP dial %s: %w", addr, err)
	}

	user := f.Creds.Username
	if user == "" {
		user = "anonymous"
	}
	if err := conn.Login(user, f.Creds.Password); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("FTP login: %w", err)
	}

	return conn, nil
}

func (f *FTPStorage) Download(ctx context.Context, localPath string) error {
	remotePath, err := f.parsePath()
	if err != nil {
		return err
	}

	conn, err := f.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Quit()

	resp, err := conn.Retr(remotePath)
	if err != nil {
		return fmt.Errorf("FTP retrieve %s: %w", remotePath, err)
	}
	defer resp.Close()

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write local file: %w", writeErr)
			}
		}
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			}
			return fmt.Errorf("read from FTP: %w", readErr)
		}
	}

	return nil
}

func (f *FTPStorage) Upload(ctx context.Context, localPath string) error {
	remotePath, err := f.parsePath()
	if err != nil {
		return err
	}

	conn, err := f.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Quit()

	// Create parent directories
	dir := path.Dir(remotePath)
	if dir != "" && dir != "." && dir != "/" {
		conn.MakeDir(dir) // best-effort, may already exist
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer file.Close()

	if err := conn.Stor(remotePath, file); err != nil {
		return fmt.Errorf("FTP store %s: %w", remotePath, err)
	}

	return nil
}
