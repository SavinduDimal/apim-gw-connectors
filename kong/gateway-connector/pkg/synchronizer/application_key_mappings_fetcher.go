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

package synchronizer

import (
	"time"

	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventhub "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/types"
	sync "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/synchronizer"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"
	logger "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/loggers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FetchApplicationKeyMappingsOnEvent fetches the policies from the control plane on the start up and notification event updates
func FetchApplicationKeyMappingsOnEvent(applicationUUID string, organization string, c client.Client) ([]eventhub.ApplicationKeyMapping, string) {
	logger.LoggerSynchronizer.Debugf("Starting application key mappings fetch | applicationUUID:%s organization:%s", applicationUUID, organization)

	applicationKeyMappings, errorMsg := FetchApplicationKeyMappings(organization, c)
	if applicationKeyMappings == nil {
		return nil, errorMsg
	}

	var matchedKeys []eventhub.ApplicationKeyMapping
	for _, key := range applicationKeyMappings {
		if key.ApplicationUUID == applicationUUID {
			matchedKeys = append(matchedKeys, key)
		}
	}

	logger.LoggerSynchronizer.Debugf("Filtered application key mappings for application %s - Total fetched: %d, Matching: %d",
		applicationUUID, len(applicationKeyMappings), len(matchedKeys))

	return matchedKeys, errorMsg
}

// FetchApplicationKeyMappings fetches the policies from the control plane on the start up and notification event updates
func FetchApplicationKeyMappings(organization string, c client.Client) ([]eventhub.ApplicationKeyMapping, string) {
	logger.LoggerSynchronizer.Debugf("Starting application key mappings fetch")

	conf, errReadConfig := config.ReadConfigs()
	if errReadConfig != nil {
		logger.LoggerSynchronizer.Errorf("Error reading configs: %v", errReadConfig)
	}

	applicationKeyMappings, errorMsg := sync.FetchApplicationKeyMappings(organization)
	if applicationKeyMappings == nil {
		return nil, errorMsg
	}

	if len(applicationKeyMappings) == 0 && errorMsg != constants.EmptyString {
		logger.LoggerSynchronizer.Warnf("Error fetching application key mappings in retry attempt %d : %s", retryAttempt, errorMsg)
		go retryApplicationKeyMappingsData(organization, conf, errorMsg, c)
		return nil, errorMsg
	}

	return applicationKeyMappings, errorMsg
}

func retryApplicationKeyMappingsData(organization string, conf *config.Config, errorMessage string, c client.Client) {
	logger.LoggerSynchronizer.Debugf("Time Duration for retrying: %v",
		conf.ControlPlane.RetryInterval*time.Second)
	time.Sleep(conf.ControlPlane.RetryInterval * time.Second)
	FetchApplicationKeyMappings(organization, c)
	retryAttempt++
	if retryAttempt > constants.MaxRetries {
		logger.LoggerSynchronizer.Error(errorMessage)
		return
	}
}
