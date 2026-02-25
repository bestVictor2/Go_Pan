package config

import "sync"

// StorageConfig holds storage and migration settings.
type StorageConfig struct {
	MinioClusters       []MinioClusterConfig `json:"minio_clusters"`        // 所有可用的 minio 集群/节点
	ReplicaCount        int                  `json:"replica_count"`         // 每份对象存储几份副本
	LoadBalanceStrategy string               `json:"load_balance_strategy"` // round_robin, least_conn, hash 如何选择存储节点
	EnableSharding      bool                 `json:"enable_sharding"`       //是否切分大文件
	ShardSize           int64                `json:"shard_size"`            // 切分大文件大小
	MigrationThreshold  int                  `json:"migration_threshold"`   // 磁盘使用率达到多少启动迁移
}

// MinioClusterConfig describes a MinIO cluster node.
type MinioClusterConfig struct { // 单个 Minio 节点
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
// 目前仍是单 Minio 架构
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
