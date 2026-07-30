package backup

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func Unbundle(root string, input io.Reader, maxBytes int64) (Summary, error) {
	if !filepath.IsAbs(root) || maxBytes <= 0 {
		return Summary{}, fmt.Errorf("invalid bundle destination")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Summary{}, fmt.Errorf("create bundle destination: %w", err)
	}
	partial := filepath.Join(root, ".upload-"+uuid.NewString())
	if err := os.Mkdir(partial, 0o700); err != nil {
		return Summary{}, fmt.Errorf("create bundle staging: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(partial)
		}
	}()

	reader := tar.NewReader(input)
	var topLevel string
	var total int64
	seen := make(map[string]struct{})
	allowed := map[string]struct{}{
		"manifest.json": {}, "SHA256SUMS": {}, "COMPLETE": {},
		"grom-data.tar": {}, "signing-certs.tar": {}, "registry-data.tar": {},
		"distribution-config.yml": {},
	}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Summary{}, fmt.Errorf("read backup bundle: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			return Summary{}, fmt.Errorf("backup bundle contains an unsupported entry")
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(header.Name)))
		parts := strings.Split(clean, "/")
		if clean != header.Name || len(parts) != 2 || !strings.HasPrefix(parts[0], "grom-backup-") {
			return Summary{}, fmt.Errorf("backup bundle contains an unsafe path")
		}
		if topLevel == "" {
			topLevel = parts[0]
		}
		if parts[0] != topLevel {
			return Summary{}, fmt.Errorf("backup bundle contains multiple backup sets")
		}
		if _, ok := allowed[parts[1]]; !ok {
			return Summary{}, fmt.Errorf("backup bundle contains an undeclared payload")
		}
		if _, duplicate := seen[parts[1]]; duplicate {
			return Summary{}, fmt.Errorf("backup bundle contains a duplicate payload")
		}
		seen[parts[1]] = struct{}{}
		if header.Size < 0 || total+header.Size > maxBytes {
			return Summary{}, fmt.Errorf("backup bundle exceeds the configured upload limit")
		}
		total += header.Size
		target := filepath.Join(partial, parts[1])
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return Summary{}, fmt.Errorf("create uploaded payload: %w", err)
		}
		_, copyErr := io.CopyN(output, reader, header.Size)
		syncErr := output.Sync()
		closeErr := output.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil {
			return Summary{}, fmt.Errorf("write uploaded payload")
		}
	}
	if topLevel == "" || len(seen) != len(allowed) {
		return Summary{}, fmt.Errorf("backup bundle is incomplete")
	}
	if _, err := Inspect(partial); err != nil {
		return Summary{}, err
	}
	if err := syncDirectory(partial); err != nil {
		return Summary{}, err
	}
	final := filepath.Join(root, topLevel)
	if _, err := os.Lstat(final); !os.IsNotExist(err) {
		return Summary{}, fmt.Errorf("backup set already exists")
	}
	if err := os.Rename(partial, final); err != nil {
		return Summary{}, fmt.Errorf("publish uploaded backup: %w", err)
	}
	published = true
	inspection, err := Inspect(final)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		BackupID:    inspection.Manifest.BackupID,
		CreatedAt:   inspection.Manifest.CreatedAt.Format(time.RFC3339),
		GromVersion: inspection.Manifest.GromVersion,
		TotalBytes:  inspection.TotalBytes,
		Path:        final,
	}, nil
}
