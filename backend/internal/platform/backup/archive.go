package backup

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func writeArchive(source, destination string) (Component, error) {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Component{}, fmt.Errorf("create component archive: %w", err)
	}
	hash := sha256.New()
	writer := tar.NewWriter(io.MultiWriter(output, hash))
	var entries, contentBytes int64
	walkErr := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if err := validateArchivePath(name); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported in backup sources")
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported special file in backup source")
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if info.IsDir() && name != "." {
			header.Name += "/"
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		entries++
		if info.Mode().IsRegular() {
			input, err := os.Open(path)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(writer, input)
			closeErr := input.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			contentBytes += written
		}
		return nil
	})
	closeTarErr := writer.Close()
	syncErr := output.Sync()
	closeErr := output.Close()
	if walkErr != nil {
		return Component{}, fmt.Errorf("archive source: %w", walkErr)
	}
	if closeTarErr != nil {
		return Component{}, fmt.Errorf("finalize archive: %w", closeTarErr)
	}
	if syncErr != nil {
		return Component{}, fmt.Errorf("sync archive: %w", syncErr)
	}
	if closeErr != nil {
		return Component{}, fmt.Errorf("close archive: %w", closeErr)
	}
	info, err := os.Stat(destination)
	if err != nil {
		return Component{}, err
	}
	return Component{
		Bytes: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)),
		EntryCount: entries, ContentBytes: contentBytes,
	}, nil
}

func applyDirectoryMetadata(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("read restored root metadata: %w", err)
	}
	if err := os.Chmod(target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("restore volume root permissions: %w", err)
	}
	if os.Geteuid() == 0 {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("read restored root ownership: %w", err)
		}
		if err := os.Chown(target, header.Uid, header.Gid); err != nil {
			return fmt.Errorf("restore volume root ownership: %w", err)
		}
	}
	return nil
}

func extractArchive(archivePath, staging string, expected Component) error {
	input, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open component archive: %w", err)
	}
	defer func() { _ = input.Close() }()
	reader := tar.NewReader(input)
	seen := make(map[string]struct{})
	var entries, contentBytes int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read component archive: %w", err)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if err := validateArchivePath(name); err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("component archive contains a duplicate path")
		}
		seen[name] = struct{}{}
		target := filepath.Join(staging, filepath.FromSlash(name))
		if !pathWithin(staging, target) {
			return fmt.Errorf("component archive path escapes staging")
		}
		mode := os.FileMode(header.Mode) & os.ModePerm
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode); err != nil {
				return fmt.Errorf("create restored directory: %w", err)
			}
		case tar.TypeReg:
			if header.Size < 0 {
				return fmt.Errorf("component archive contains an invalid file size")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("create restored parent: %w", err)
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("create restored file: %w", err)
			}
			written, copyErr := io.CopyN(output, reader, header.Size)
			syncErr := output.Sync()
			closeErr := output.Close()
			if copyErr != nil {
				return fmt.Errorf("extract restored file: %w", copyErr)
			}
			if syncErr != nil || closeErr != nil {
				return fmt.Errorf("sync restored file")
			}
			contentBytes += written
		default:
			return fmt.Errorf("component archive contains an unsupported entry type")
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("restore permissions: %w", err)
		}
		if os.Geteuid() == 0 {
			if err := os.Chown(target, header.Uid, header.Gid); err != nil {
				return fmt.Errorf("restore ownership: %w", err)
			}
		}
		entries++
	}
	if entries != expected.EntryCount || contentBytes != expected.ContentBytes {
		return fmt.Errorf("component archive measurements do not match manifest")
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || len(name) > MaxArchivePath || filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return fmt.Errorf("component archive contains an invalid path")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if clean != name || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("component archive contains an unsafe path")
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
