package foundation

import "time"

// AccountedStorageUsage is logical OCI descriptor usage. It deliberately is
// not filesystem usage: blobs shared by separate projects count in each
// project, while descriptors shared inside the scope count once.
type AccountedStorageUsage struct {
	Status         string     `json:"status"`
	AccountedBytes *int64     `json:"accountedBytes"`
	ReconciledAt   *time.Time `json:"reconciledAt"`
}

func PendingStorageUsage() AccountedStorageUsage {
	return AccountedStorageUsage{Status: "pending"}
}
