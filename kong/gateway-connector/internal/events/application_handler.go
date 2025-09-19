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

package events

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventConstants "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/constants"
	msg "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/messaging"
	kongConstants "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"
	internalk8sClient "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/k8sClient"
	logger "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/loggers"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/utils"
	kongMgtServer "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/managementserver"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/synchronizer"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/transformer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleApplicationEvents to process application related events
func HandleApplicationEvents(data []byte, eventType string, c client.Client) {
	logger.LoggerEvents.Infof("Processing application event with EventType: %s, data length: %d bytes", eventType, len(data))

	conf, _ := config.ReadConfigs()

	switch {
	case strings.EqualFold(eventConstants.ApplicationRegistration, eventType):
		handleApplicationRegistration(data, c, conf)
	case strings.EqualFold(eventConstants.RemoveApplicationKeyMapping, eventType):
		handleRemoveApplicationKeyMapping(data, c, conf)
	default:
		handleApplicationEvent(data, c, conf)
	}
}

// handleApplicationRegistration processes application registration events
func handleApplicationRegistration(data []byte, c client.Client, conf *config.Config) {
	var applicationRegistrationEvent msg.ApplicationRegistrationEvent
	if err := json.Unmarshal(data, &applicationRegistrationEvent); err != nil {
		logger.LoggerEvents.Errorf("%s: %v", kongConstants.UnmarshalErrorApplication, err)
		return
	}

	consumerName := transformer.GenerateConsumerName(applicationRegistrationEvent.ApplicationUUID, strings.ToLower(applicationRegistrationEvent.KeyType))
	consumer := internalk8sClient.GetKongConsumerCR(consumerName, c, conf)

	if consumer == nil {
		logger.LoggerEvents.Debugf("Application Registration consumer not found for application UUID %s, skipping creation",
			applicationRegistrationEvent.ApplicationUUID)
		return
	}

	if !belongsToTenant(applicationRegistrationEvent.TenantDomain) {
		logger.LoggerEvents.Debugf("Application Registration event is dropped due to having non related tenantDomain : %s",
			applicationRegistrationEvent.TenantDomain)
		return
	}

	if !kongMgtServer.IsApplicationProcessed(applicationRegistrationEvent.ApplicationUUID) {
		logger.LoggerEvents.Infof("Application %s is not processed. skipping application registration event",
			applicationRegistrationEvent.ApplicationUUID)
		return
	}

	synchronizer.ProcessApplicationRegistration(
		applicationRegistrationEvent.ApplicationUUID,
		applicationRegistrationEvent.ConsumerKey,
		applicationRegistrationEvent.KeyManager,
		applicationRegistrationEvent.TenantDomain,
		strings.ToLower(applicationRegistrationEvent.KeyType),
		c,
		conf,
	)
}

// handleRemoveApplicationKeyMapping processes application key removal events
func handleRemoveApplicationKeyMapping(data []byte, c client.Client, conf *config.Config) {
	var applicationRegistrationEvent msg.ApplicationRegistrationEvent
	if err := json.Unmarshal(data, &applicationRegistrationEvent); err != nil {
		logger.LoggerEvents.Errorf("Error occurred while unmarshalling Application Registration event data %v", err)
		return
	}

	if !belongsToTenant(applicationRegistrationEvent.TenantDomain) {
		logger.LoggerEvents.Debugf("Application Registration event is dropped due to having non related tenantDomain : %s",
			applicationRegistrationEvent.TenantDomain)
		return
	}

	logger.LoggerEvents.Debugf("Received Remove Application Key Mapping Event: %+v", applicationRegistrationEvent)

	if !kongMgtServer.IsApplicationProcessed(applicationRegistrationEvent.ApplicationUUID) {
		logger.LoggerEvents.Infof("Application %s is not processed. skipping remove application key mapping event",
			applicationRegistrationEvent.ApplicationUUID)
		return
	}

	jwtCredentialSecretName := transformer.GenerateSecretName(
		applicationRegistrationEvent.ApplicationUUID,
		applicationRegistrationEvent.ConsumerKey,
		kongConstants.JWTCredentialType)
	removeCredentials := []string{jwtCredentialSecretName}

	utils.RetryKongCRUpdate(func() error {
		internalk8sClient.UpdateKongConsumerCredential(
			applicationRegistrationEvent.ApplicationUUID,
			kongConstants.EmptyString,
			c, conf, nil, removeCredentials)
		return nil
	}, kongConstants.RemoveApplicationKeyTaskName, kongConstants.MaxRetries)

	internalk8sClient.UnDeploySecretCR(jwtCredentialSecretName, c, conf)
}

// handleApplicationEvent processes general application events
func handleApplicationEvent(data []byte, c client.Client, conf *config.Config) {
	var applicationEvent msg.ApplicationEvent
	if err := json.Unmarshal(data, &applicationEvent); err != nil {
		logger.LoggerEvents.Errorf("Error occurred while unmarshalling Application event data %v", err)
		return
	}

	if !belongsToTenant(applicationEvent.TenantDomain) {
		logger.LoggerEvents.Debugf("Application event for the Application : %s (with uuid %s) is dropped due to having non related tenantDomain : %s",
			applicationEvent.ApplicationName, applicationEvent.UUID, applicationEvent.TenantDomain)
		return
	}

	if isLaterEvent(applicationListTimeStampMap, fmt.Sprint(applicationEvent.ApplicationID), applicationEvent.TimeStamp) {
		return
	}

	logger.LoggerEvents.Debugf("Received Application Event: %+v", applicationEvent)

	switch applicationEvent.Event.Type {
	case eventConstants.ApplicationCreate:
		logger.LoggerEvents.Debugf("Application Create for application UUID %s", applicationEvent.UUID)
	case eventConstants.ApplicationUpdate:
		logger.LoggerEvents.Debugf("Application Update for application UUID %s", applicationEvent.UUID)
	case eventConstants.ApplicationDelete:
		logger.LoggerEvents.Debugf("Application Delete for application UUID %s", applicationEvent.UUID)
		if !kongMgtServer.IsApplicationProcessed(applicationEvent.UUID) {
			logger.LoggerEvents.Infof("Application %s is not processed. skipping deletion event",
				applicationEvent.UUID)
			return
		}
		internalk8sClient.UndeployAPPCRs(applicationEvent.UUID, c)
		kongMgtServer.RemoveProcessedApplication(applicationEvent.UUID)
	default:
		logger.LoggerEvents.Warnf("Application Event Type '%s' is not recognized for the Event under Application UUID %s",
			applicationEvent.Event.Type, applicationEvent.UUID)
	}
}
