/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

/*
 * Package "synchronizer" contains artifacts relate to fetching APIs and
 * API related updates from the control plane event-hub.
 * This file contains functions to retrieve APIs and API updates.
 */

package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"

	logger "github.com/wso2-extensions/apim-gw-connectors/apk/gateway-connector/internal/loggers"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventhubTypes "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/types"
)

// GetEnvLabel returns the environment label from the config
func GetEnvLabel() string {
	envID := "Default" // fallback default
	if conf, err := config.ReadConfigs(); err == nil && len(conf.ControlPlane.EnvironmentLabels) > 0 {
		envID = conf.ControlPlane.EnvironmentLabels[0] // Use the first environment label
	}
	return envID
}

// Helper function to extract hostname and port from URL
func ExtractHostnameAndPort(serverURL string) (string, int, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", -1, err
	}
	host, port := u.Hostname(), u.Port()
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return "", -1, err
	}
	return host, portInt, nil
}

// GetSha1Value is a helper function to get the sha1 value of a given input
func GetSha1Value(input string) string {
	hasher := sha1.New()
	hasher.Write([]byte(input))
	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

// GetBackendConfigForKM returns the backend name, port and the namespace for the backends created for the KMs
func GetBackendConfigForKM(km eventhubTypes.ResolvedKeyManager) (string, string, int, string) {
	agentNS := "default"
	conf, err := config.ReadConfigs()
	if err != nil {
		logger.LoggerSynchronizer.Errorf("Error reading configs: %v", err)
	}
	if conf.DataPlane.Namespace != "" {
		agentNS = conf.DataPlane.Namespace // Use the ns which the dataplane is running
		// fmt.Printf("\nDataplane Namespace: %s\n", agentNS)
	}

	backendName := GetSha1Value(fmt.Sprintf("%s-%s", km.Name, km.Organization))
	// Extract server URL from the Key Manager config
	jwksURL := km.KeyManagerConfig.CertificateValue
	hostname, port, err := ExtractHostnameAndPort(jwksURL)
	if err != nil {
		logger.LoggerSynchronizer.Errorf("Failed to extract hostname and port from the key manager config %+v", err.Error())
	}
	return backendName, hostname, port, agentNS
}

// HashLast50SHA1 returns the last 50 characters of the SHA-1 hash of the input string.
// Since SHA-1 is only 40 hex chars, this will just return the whole hash.
func HashLast50SHA1(input string) string {
	hash := sha1.Sum([]byte(input))
	hexStr := hex.EncodeToString(hash[:]) // SHA-1 produces 40 hex chars
	if len(hexStr) <= 50 {
		return hexStr
	}
	return hexStr[len(hexStr)-50:]
}

// CreateAIProviderName creates a unique name for the AI provider based on the provider name and API version.
func CreateAIProviderName(providerName string, ProviderAPIVersion string) string {
	// Create a unique name for the AI provider by hashing the provider name
	// This ensures that the name is unique and does not exceed length limits
	return "ai-provider-" + HashLast50SHA1(providerName+ProviderAPIVersion)
}

// CreateSubscriptionPolicyName creates a unique name for the subscription policy based on the policy name and organization.
func CreateSubscriptionPolicyName(policyName string, organization string) string {
	return "subscription-" + HashLast50SHA1(policyName+organization)
}