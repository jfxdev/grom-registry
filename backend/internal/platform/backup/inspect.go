package backup

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Inspection struct {
	Manifest   Manifest
	TotalBytes int64
}

func Inspect(path string) (Inspection, error) {
	if !filepath.IsAbs(path) {
		return Inspection{}, fmt.Errorf("backup path must be absolute")
	}
	info, err := os.Stat(filepath.Join(path, "COMPLETE"))
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return Inspection{}, fmt.Errorf("backup completion marker is missing")
	}
	manifest, raw, err := readManifest(path)
	if err != nil {
		return Inspection{}, err
	}
	allowed := map[string]struct{}{"manifest.json": {}, "SHA256SUMS": {}, "COMPLETE": {}}
	expectedHashes := map[string]string{"manifest.json": checksumBytes(raw)}
	var total int64
	for _, component := range manifest.Components {
		allowed[component.File] = struct{}{}
		expectedHashes[component.File] = component.SHA256
		componentPath := filepath.Join(path, component.File)
		info, err := os.Lstat(componentPath)
		if err != nil || !info.Mode().IsRegular() {
			return Inspection{}, fmt.Errorf("backup component %q is missing or invalid", component.Name)
		}
		if info.Size() != component.Bytes {
			return Inspection{}, fmt.Errorf("backup component %q size does not match manifest", component.Name)
		}
		hash, err := checksumFile(componentPath)
		if err != nil {
			return Inspection{}, fmt.Errorf("checksum component %q: %w", component.Name, err)
		}
		if hash != component.SHA256 {
			return Inspection{}, fmt.Errorf("backup component %q checksum does not match manifest", component.Name)
		}
		total += component.Bytes
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return Inspection{}, fmt.Errorf("list backup set: %w", err)
	}
	for _, entry := range entries {
		if _, ok := allowed[entry.Name()]; !ok {
			return Inspection{}, fmt.Errorf("backup set contains an undeclared file")
		}
	}
	sums, err := readChecksumFile(filepath.Join(path, "SHA256SUMS"))
	if err != nil {
		return Inspection{}, err
	}
	if len(sums) != len(expectedHashes) {
		return Inspection{}, fmt.Errorf("checksum file does not declare the exact payload set")
	}
	for name, expected := range expectedHashes {
		if sums[name] != expected {
			return Inspection{}, fmt.Errorf("checksum file does not match %q", name)
		}
	}
	return Inspection{Manifest: manifest, TotalBytes: total}, nil
}

func readChecksumFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open checksum file: %w", err)
	}
	defer func() { _ = file.Close() }()
	result := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(file, MaxManifestSize+1))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, fmt.Errorf("checksum file contains an invalid line")
		}
		hash, name := line[:64], line[66:]
		if !isLowerHex(hash) || name == "" || strings.ContainsAny(name, "/\\") {
			return nil, fmt.Errorf("checksum file contains an invalid entry")
		}
		if _, duplicate := result[name]; duplicate {
			return nil, fmt.Errorf("checksum file contains a duplicate entry")
		}
		result[name] = hash
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksum file: %w", err)
	}
	return result, nil
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func checksumBytes(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
