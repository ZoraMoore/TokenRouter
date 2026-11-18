package provider

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

// ensureQoderMachineCredentials 为新建 Qoder 提供商补齐并持久化站点对应的稳定机器身份。
// direct token 提供商必须由调用方提供 machine_id；国内站不生成额外机器字段。
func ensureQoderMachineCredentials(provider *provider.Record) {
	if provider == nil {
		return
	}
	if provider.Credentials == nil {
		provider.Credentials = make(map[string]any)
	}
	pat := strings.TrimSpace(provider.GetCredential("pat"))
	directToken := strings.TrimSpace(provider.GetCredential("security_oauth_token"))
	machineID := strings.TrimSpace(provider.GetCredential("machine_id"))
	if pat == "" && (directToken == "" || machineID == "") {
		return
	}
	site, err := qoderSiteForRecord(provider)
	if err != nil {
		site = qoder.SiteGlobal
	}
	if site == qoder.SiteCN {
		// 国内客户端只持久化 machine_id，并清理旧版本曾写入的随机机器字段。
		delete(provider.Credentials, "machine_token")
		delete(provider.Credentials, "machine_type")
		if pat != "" && machineID == "" {
			provider.Credentials["machine_id"] = qoder.NewMachineForSite(site).MachineID
		}
		return
	}
	machine := qoder.NewMachineForSite(site)
	if machineID != "" && strings.TrimSpace(provider.GetCredential("machine_token")) != "" &&
		strings.TrimSpace(provider.GetCredential("machine_type")) != "" {
		return
	}
	if pat != "" && machineID == "" {
		provider.Credentials["machine_id"] = machine.MachineID
	}
	if strings.TrimSpace(provider.GetCredential("machine_token")) == "" {
		provider.Credentials["machine_token"] = machine.MachineToken
	}
	if strings.TrimSpace(provider.GetCredential("machine_type")) == "" {
		provider.Credentials["machine_type"] = machine.MachineType
	}
}
