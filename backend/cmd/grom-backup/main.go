package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jfxdev/grom/backend/internal/platform/backup"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "grom-backup:", err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: grom-backup create|inspect|restore")
	}
	switch arguments[0] {
	case "recovery":
		flags := flag.NewFlagSet("recovery", flag.ContinueOnError)
		listen := flags.String("listen", ":8080", "recovery UI listen address")
		backupRoot := flags.String("backup-root", "/backups", "verified backup storage")
		gromData := flags.String("grom-data-target", "/target/grom-data", "empty Grom data target")
		signingCerts := flags.String("signing-certs-target", "/target/signing-certs", "empty signing target")
		registryData := flags.String("registry-data-target", "/target/registry-data", "empty registry target")
		distributionConfig := flags.String("distribution-config-target", "/target/distribution-config/config.yml", "recovered Distribution configuration")
		maxUpload := flags.Int64("max-upload-bytes", 1<<40, "maximum uncompressed backup payload bytes")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("recovery does not accept positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return backup.ServeRecovery(ctx, backup.RecoveryOptions{
			ListenAddress: *listen, BackupRoot: *backupRoot,
			GromDataTarget: *gromData, SigningCertsTarget: *signingCerts,
			RegistryDataTarget: *registryData, DistributionConfigTarget: *distributionConfig,
			TargetGromVersion: version, MaxUploadBytes: *maxUpload,
		})
	case "agent":
		flags := flag.NewFlagSet("agent", flag.ContinueOnError)
		socket := flags.String("socket", "/run/grom-backup/agent.sock", "Unix socket path")
		destination := flags.String("destination", "/backups", "backup storage root")
		gromData := flags.String("grom-data", "/source/grom-data", "Grom data source")
		signingCerts := flags.String("signing-certs", "/source/signing-certs", "signing source")
		registryData := flags.String("registry-data", "/source/registry-data", "registry source")
		distributionConfig := flags.String("distribution-config", "/source/distribution-config.yml", "Distribution configuration source")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("agent does not accept positional arguments")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return backup.ServeAgent(ctx, backup.AgentOptions{
			SocketPath: *socket, DestinationRoot: *destination,
			GromData: *gromData, SigningCerts: *signingCerts,
			RegistryData: *registryData, DistributionConfig: *distributionConfig,
		})
	case "estimate":
		flags := flag.NewFlagSet("estimate", flag.ContinueOnError)
		gromData := flags.String("grom-data", "/source/grom-data", "Grom data source")
		signingCerts := flags.String("signing-certs", "/source/signing-certs", "signing source")
		registryData := flags.String("registry-data", "/source/registry-data", "registry source")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("estimate does not accept positional arguments")
		}
		results, err := backup.Estimate(map[string]string{
			backup.ComponentGromData:     *gromData,
			backup.ComponentSigningCerts: *signingCerts,
			backup.ComponentRegistryData: *registryData,
		})
		if err != nil {
			return err
		}
		var total int64
		for _, result := range results {
			if _, err := fmt.Fprintf(output, "%s_bytes=%d\n", result.Name, result.Bytes); err != nil {
				return fmt.Errorf("write estimate output: %w", err)
			}
			total += result.Bytes
		}
		if _, err := fmt.Fprintf(output, "estimated_total_bytes=%d\n", total); err != nil {
			return fmt.Errorf("write estimate output: %w", err)
		}
		return nil
	case "create":
		flags := flag.NewFlagSet("create", flag.ContinueOnError)
		destination := flags.String("destination", "", "absolute backup destination directory")
		gromData := flags.String("grom-data", "/source/grom-data", "Grom data source")
		signingCerts := flags.String("signing-certs", "/source/signing-certs", "signing source")
		registryData := flags.String("registry-data", "/source/registry-data", "registry source")
		distributionConfig := flags.String("distribution-config", "/source/distribution-config.yml", "Distribution configuration source")
		profile := flags.String("deployment-profile", "", "source deployment profile")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *destination == "" || *profile == "" {
			return fmt.Errorf("create requires --destination and --deployment-profile")
		}
		started := time.Now()
		inspection, path, err := backup.Create(backup.CreateOptions{
			DestinationRoot: *destination, GromData: *gromData, SigningCerts: *signingCerts,
			RegistryData: *registryData, DistributionConfig: *distributionConfig,
			GromVersion: version, DeploymentProfile: *profile,
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "backup_path=%s\nbackup_id=%s\ncreated_at=%s\ntotal_bytes=%d\nduration=%s\n",
			path, inspection.Manifest.BackupID, inspection.Manifest.CreatedAt.Format(time.RFC3339),
			inspection.TotalBytes, time.Since(started).Round(time.Millisecond)); err != nil {
			return fmt.Errorf("write create output: %w", err)
		}
		return nil
	case "inspect":
		flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
		path := flags.String("path", "", "absolute backup set path")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *path == "" {
			return fmt.Errorf("inspect requires --path")
		}
		inspection, err := backup.Inspect(*path)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "backup_id=%s\ncreated_at=%s\ngrom_version=%s\ntotal_bytes=%d\nstatus=complete\n",
			inspection.Manifest.BackupID, inspection.Manifest.CreatedAt.Format(time.RFC3339),
			inspection.Manifest.GromVersion, inspection.TotalBytes); err != nil {
			return fmt.Errorf("write inspect output: %w", err)
		}
		return nil
	case "restore":
		flags := flag.NewFlagSet("restore", flag.ContinueOnError)
		path := flags.String("path", "", "absolute backup set path")
		gromData := flags.String("grom-data-target", "/target/grom-data", "empty Grom data target")
		signingCerts := flags.String("signing-certs-target", "/target/signing-certs", "empty signing target")
		registryData := flags.String("registry-data-target", "/target/registry-data", "empty registry target")
		distributionConfig := flags.String("distribution-config-target", "", "new recovered Distribution configuration file")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *path == "" || *distributionConfig == "" {
			return fmt.Errorf("restore requires --path and --distribution-config-target")
		}
		started := time.Now()
		inspection, err := backup.Restore(backup.RestoreOptions{
			BackupPath: *path, GromDataTarget: *gromData, SigningCertsTarget: *signingCerts,
			RegistryDataTarget: *registryData, DistributionConfigTarget: *distributionConfig,
			TargetGromVersion: version,
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "backup_id=%s\nsource_version=%s\ntotal_bytes=%d\nduration=%s\nstatus=restored\n",
			inspection.Manifest.BackupID, inspection.Manifest.GromVersion,
			inspection.TotalBytes, time.Since(started).Round(time.Millisecond)); err != nil {
			return fmt.Errorf("write restore output: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}
