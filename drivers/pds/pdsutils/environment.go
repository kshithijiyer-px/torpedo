package pdsutils

import (
	"os"

	log "github.com/sirupsen/logrus"
)

const (
	envControlPlaneURL    = "CONTROL_PLANE_URL"
	envPDSTestAccountName = "TEST_ACCOUNT_NAME"
	envTargetKubeconfig   = "TARGET_KUBECONFIG"
	envUsername           = "PDS_USERNAME"
	envPassword           = "PDS_PASSWORD"
	envPDSClientSecret    = "PDS_CLIENT_SECRET"
	envPDSClientID        = "PDS_CLIENT_ID"
	envPDSISSUERURL       = "PDS_ISSUER_URL"
	envClusterType        = "CLUSTER_TYPE"
)

// Environment lhasha
type Environment struct {
	PDSControlPlaneURL   string
	PDSTestAccountName   string
	PDSTargetKUBECONFIG  string
	PDSUsername          string
	PDSPassword          string
	PDSIssuerURL         string
	PDSClientID          string
	PDSClientSecret      string
	PDSTargetClusterType string
}

// MustHaveEnvVariables return emnvironment variables.
func MustHaveEnvVariables() Environment {
	return Environment{
		PDSControlPlaneURL:   mustGetEnvVariable(envControlPlaneURL),
		PDSTestAccountName:   mustGetEnvVariable(envPDSTestAccountName),
		PDSTargetKUBECONFIG:  mustGetEnvVariable(envTargetKubeconfig),
		PDSUsername:          mustGetEnvVariable(envUsername),
		PDSPassword:          mustGetEnvVariable(envPassword),
		PDSIssuerURL:         mustGetEnvVariable(envPDSISSUERURL),
		PDSClientID:          mustGetEnvVariable(envPDSClientID),
		PDSClientSecret:      mustGetEnvVariable(envPDSClientSecret),
		PDSTargetClusterType: mustGetEnvVariable(envClusterType),
	}
}

// mustGetEnvVariable return environment variable.
func mustGetEnvVariable(key string) string {
	value, isExist := os.LookupEnv(key)
	if !isExist {
		log.Panicf("Key: %v doesn't exist", key)
	}
	return value
}