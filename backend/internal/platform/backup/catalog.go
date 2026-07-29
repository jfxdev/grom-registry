package backup

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		inspection, err := Inspect(path)
		if err != nil {
			continue
		}
		result = append(result, Summary{
			BackupID: inspection.Manifest.BackupID, CreatedAt: inspection.Manifest.CreatedAt.Format(time.RFC3339),
			GromVersion: inspection.Manifest.GromVersion, TotalBytes: inspection.TotalBytes, Path: path,
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
	items := append([]Summary(nil), summaries[start:end]...)
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

func Bundle(root, backupID string, output io.Writer) (string, error) {
	summary, err := findSummary(root, backupID)
	if err != nil {
		return "", err
	}
	if _, err := Inspect(summary.Path); err != nil {
		return "", err
	}
	writer := tar.NewWriter(output)
	defer writer.Close()
	entries, err := os.ReadDir(summary.Path)
	if err != nil {
		return "", fmt.Errorf("list backup set: %w", err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("backup set contains a non-regular payload")
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return "", err
		}
		header.Name = filepath.ToSlash(filepath.Join(filepath.Base(summary.Path), entry.Name()))
		if err := writer.WriteHeader(header); err != nil {
			return "", err
		}
		input, err := os.Open(filepath.Join(summary.Path, entry.Name()))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil || closeErr != nil {
			return "", fmt.Errorf("stream backup payload")
		}
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finalize backup bundle: %w", err)
	}
	return filepath.Base(summary.Path) + ".tar", nil
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
