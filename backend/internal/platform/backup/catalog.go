package backup

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const PageSize = 5

var ErrInvalidCursor = errors.New("invalid backup cursor")

type Summary struct {
	BackupID    string `json:"backupId"`
	CreatedAt   string `json:"createdAt"`
	GromVersion string `json:"gromVersion"`
	TotalBytes  int64  `json:"totalBytes"`
	Path        string `json:"-"`
}

type Page struct {
	Items      []Summary
	Total      int
	NextCursor string
}

type pageCursor struct {
	CreatedAt string `json:"createdAt"`
	BackupID  string `json:"backupId"`
}

func List(root string) ([]Summary, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("backup root must be absolute")
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backup root: %w", err)
	}
	result := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "grom-backup-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		marker, err := os.Stat(filepath.Join(path, "COMPLETE"))
		if err != nil || !marker.Mode().IsRegular() || marker.Size() != 0 {
			slog.Warn(
				"skipping invalid backup set",
				"backup_set", entry.Name(),
				"reason", "completion marker is missing or invalid",
			)
			continue
		}
		manifest, _, err := readManifest(path)
		if err != nil {
			slog.Warn(
				"skipping invalid backup set",
				"backup_set", entry.Name(),
				"reason", "manifest is missing or invalid",
			)
			continue
		}
		var totalBytes int64
		for _, component := range manifest.Components {
			totalBytes += component.Bytes
		}
		result = append(result, Summary{
			BackupID: manifest.BackupID, CreatedAt: manifest.CreatedAt.Format(time.RFC3339),
			GromVersion: manifest.GromVersion, TotalBytes: totalBytes, Path: path,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, result[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339, result[j].CreatedAt)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		if result[i].CreatedAt != result[j].CreatedAt {
			return result[i].CreatedAt > result[j].CreatedAt
		}
		return result[i].BackupID > result[j].BackupID
	})
	return result, nil
}

func Paginate(summaries []Summary, cursor string) (Page, error) {
	start := 0
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return Page{}, ErrInvalidCursor
		}
		var position pageCursor
		decoder := json.NewDecoder(strings.NewReader(string(decoded)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&position); err != nil {
			return Page{}, ErrInvalidCursor
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Page{}, ErrInvalidCursor
		}
		cursorTime, err := time.Parse(time.RFC3339, position.CreatedAt)
		if err != nil || uuid.Validate(position.BackupID) != nil {
			return Page{}, ErrInvalidCursor
		}
		start = len(summaries)
		for index, summary := range summaries {
			summaryTime, err := time.Parse(time.RFC3339, summary.CreatedAt)
			if err != nil {
				return Page{}, ErrInvalidCursor
			}
			if summaryTime.Before(cursorTime) ||
				(summaryTime.Equal(cursorTime) && summary.BackupID < position.BackupID) {
				start = index
				break
			}
		}
	}
	end := min(start+PageSize, len(summaries))
	items := append([]Summary{}, summaries[start:end]...)
	page := Page{Items: items, Total: len(summaries)}
	if end < len(summaries) && len(items) > 0 {
		last := items[len(items)-1]
		raw, err := json.Marshal(pageCursor{CreatedAt: last.CreatedAt, BackupID: last.BackupID})
		if err != nil {
			return Page{}, fmt.Errorf("encode backup cursor: %w", err)
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

func Delete(root, backupID string) (Summary, error) {
	if !filepath.IsAbs(root) {
		return Summary{}, fmt.Errorf("backup root must be absolute")
	}
	if uuid.Validate(backupID) != nil {
		return Summary{}, os.ErrNotExist
	}
	summary, err := findSummary(root, backupID)
	if err != nil {
		return Summary{}, err
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(summary.Path)
	relative, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || relative == "." || strings.Contains(relative, string(filepath.Separator)) ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Summary{}, fmt.Errorf("backup path is outside its storage root")
	}
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return Summary{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Summary{}, fmt.Errorf("backup path is not a regular directory")
	}
	if _, err := Inspect(cleanPath); err != nil {
		return Summary{}, fmt.Errorf("refuse deletion of an invalid backup: %w", err)
	}
	tombstone := filepath.Join(cleanRoot, ".deleting-"+backupID)
	if err := os.Rename(cleanPath, tombstone); err != nil {
		return Summary{}, fmt.Errorf("stage backup deletion: %w", err)
	}
	if err := os.RemoveAll(tombstone); err != nil {
		return Summary{}, fmt.Errorf("delete staged backup: %w", err)
	}
	return summary, nil
}

type bundleEntry struct {
	name string
	info os.FileInfo
	file *os.File
}

type bundleSnapshot struct {
	filename string
	entries  []bundleEntry
}

func prepareBundle(root, backupID string) (*bundleSnapshot, error) {
	summary, err := findSummary(root, backupID)
	if err != nil {
		return nil, err
	}
	if _, err := Inspect(summary.Path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(summary.Path)
	if err != nil {
		return nil, fmt.Errorf("list backup set: %w", err)
	}
	snapshot := &bundleSnapshot{filename: filepath.Base(summary.Path) + ".tar"}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			snapshot.close()
			return nil, fmt.Errorf("backup set contains a non-regular payload")
		}
		file, err := os.Open(filepath.Join(summary.Path, entry.Name()))
		if err != nil {
			snapshot.close()
			return nil, err
		}
		snapshot.entries = append(snapshot.entries, bundleEntry{name: entry.Name(), info: info, file: file})
	}
	return snapshot, nil
}

func (snapshot *bundleSnapshot) writeTo(output io.Writer) error {
	writer := tar.NewWriter(output)
	for _, entry := range snapshot.entries {
		if _, err := entry.file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(entry.info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(strings.TrimSuffix(snapshot.filename, ".tar"), entry.name))
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if _, err := io.Copy(writer, entry.file); err != nil {
			return fmt.Errorf("stream backup payload")
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize backup bundle: %w", err)
	}
	return nil
}

func (snapshot *bundleSnapshot) close() {
	for _, entry := range snapshot.entries {
		_ = entry.file.Close()
	}
}

func Bundle(root, backupID string, output io.Writer) (string, error) {
	snapshot, err := prepareBundle(root, backupID)
	if err != nil {
		return "", err
	}
	defer snapshot.close()
	if err := snapshot.writeTo(output); err != nil {
		return "", err
	}
	return snapshot.filename, nil
}

func findSummary(root, backupID string) (Summary, error) {
	summaries, err := List(root)
	if err != nil {
		return Summary{}, err
	}
	for _, summary := range summaries {
		if summary.BackupID == backupID {
			return summary, nil
		}
	}
	return Summary{}, os.ErrNotExist
}
