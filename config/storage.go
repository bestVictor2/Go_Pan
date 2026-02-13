package config

import "sync"

// StorageConfig holds storage and migration settings.
type StorageConfig struct {
	MinioClusters       []MinioClusterConfig `json:"minio_clusters"`
	ReplicaCount        int                  `json:"replica_count"`
	LoadBalanceStrategy string               `json:"load_balance_strategy"` // round_robin, least_conn, hash
	EnableSharding      bool                 `json:"enable_sharding"`
	ShardSize           int64                `json:"shard_size"`
	MigrationThreshold  int                  `json:"migration_threshold"`
}

// MinioClusterConfig describes a MinIO cluster node.
type MinioClusterConfig struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      string `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	UseSSL    bool   `json:"use_ssl"`
	Available bool   `json:"available"`
	UsedSize  int64  `json:"used_size"`
	TotalSize int64  `json:"total_size"`
	Weight    int    `json:"weight"` // load balance weight
}

var StorageConfigInstance *StorageConfig
var storageConfigOnce sync.Once

// InitStorageConfig initializes storage config.
func InitStorageConfig() {
	storageConfigOnce.Do(func() {
		StorageConfigInstance = &StorageConfig{
			MinioClusters: []MinioClusterConfig{
				{
					Name:      "cluster1",
					Host:      getEnv("MINIO_HOST", "localhost"),
					Port:      getEnv("MINIO_PORT", "9000"),
					Username:  getEnv("MINIO_USERNAME", "minioadmin"),
					Password:  getEnv("MINIO_PASSWORD", "minioadmin"),
					UseSSL:    false,
					Available: true,
					UsedSize:  0,
					TotalSize: 100 * 1024 * 1024 * 1024, // 100GB
					Weight:    1,
				},
			},
			ReplicaCount:        2,
			LoadBalanceStrategy: "round_robin",
			EnableSharding:      true,
			ShardSize:           10 * 1024 * 1024, // 10MB
			MigrationThreshold:  80,
		}
	})
}
