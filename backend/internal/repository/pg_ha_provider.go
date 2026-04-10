package repository

import "strings"

type HaProvider string

const (
	HaProviderAuto      HaProvider = "auto"
	HaProviderCNPG      HaProvider = "cnpg"
	HaProviderPatroni   HaProvider = "patroni"
	HaProviderStreaming HaProvider = "streaming"
	HaProviderStandalone HaProvider = "standalone"
)

type HaDetection struct {
	Provider HaProvider `json:"provider"`
	DetectedBy string   `json:"detected_by,omitempty"`
}

func NormalizeHaProvider(s string) HaProvider {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cnpg", "cloudnativepg":
		return HaProviderCNPG
	case "patroni":
		return HaProviderPatroni
	case "streaming", "physical", "replication":
		return HaProviderStreaming
	case "standalone":
		return HaProviderStandalone
	case "", "auto":
		return HaProviderAuto
	default:
		return HaProviderAuto
	}
}

func DetectHaProviderAuto(clusterName string, replicationAppNames []string, hasStandbys bool) HaDetection {
	cn := strings.ToLower(strings.TrimSpace(clusterName))
	if cn != "" {
		if strings.Contains(cn, "cnpg") || strings.Contains(cn, "cloudnativepg") {
			return HaDetection{Provider: HaProviderCNPG, DetectedBy: "pg_settings.cluster_name"}
		}
		if strings.Contains(cn, "patroni") {
			return HaDetection{Provider: HaProviderPatroni, DetectedBy: "pg_settings.cluster_name"}
		}
	}
	for _, a := range replicationAppNames {
		al := strings.ToLower(a)
		if strings.Contains(al, "cnpg") || strings.Contains(al, "cloudnativepg") {
			return HaDetection{Provider: HaProviderCNPG, DetectedBy: "pg_stat_replication.application_name"}
		}
		if strings.Contains(al, "patroni") {
			return HaDetection{Provider: HaProviderPatroni, DetectedBy: "pg_stat_replication.application_name"}
		}
	}
	if hasStandbys {
		return HaDetection{Provider: HaProviderStreaming, DetectedBy: "pg_stat_replication.presence"}
	}
	return HaDetection{Provider: HaProviderStandalone, DetectedBy: "no-standbys"}
}

