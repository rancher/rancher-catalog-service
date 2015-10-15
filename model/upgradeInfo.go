package model

import "github.com/rancher/go-rancher/client"

type UpgradeInfo struct {
	client.Resource
	CurrentVersion        string            `json:"currentVersion"`
	NewVersionLinks       map[string]string `json:"newVersionLinks"`
}