package backup

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type EstimateResult struct {
	Name  string
	Bytes int64
}

func Estimate(sources map[string]string) ([]EstimateResult, error) {
	results := make([]EstimateResult, 0, len(requiredComponents))
	for _, expected := range requiredComponents[:3] {
		source, ok := sources[expected.Name]
		if !ok || !filepath.IsAbs(source) {
			return nil, fmt.Errorf("estimate source for %q must be absolute", expected.Name)
		}
		var bytes int64
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("source contains an unsupported file type")
			}
			if info.Mode().IsRegular() {
				bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("estimate %q: %w", expected.Name, err)
		}
		results = append(results, EstimateResult{Name: expected.Name, Bytes: bytes})
	}
	return results, nil
}
