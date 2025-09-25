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
	"strings"
	"time"

	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventhub "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/types"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/managementserver"
	sync "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/synchronizer"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"

	internalk8sClient "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/k8sClient"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/utils"
	logger "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/loggers"
	kongMgtServer "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/managementserver"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/transformer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SubscriptionsEndpoint          string = "internal/data/v1/subscriptions"
	SubscriptionsByAPIUUIDEndpoint string = "internal/data/v1/subscriptions?apiUUID="
)

// FetchSubscriptions will fetch subscriptions from control plane on startup
func FetchSubscriptions(apiUUID string) ([]eventhub.Subscription, string) {
	logger.LoggerSynchronizer.Debugf("Starting subscription fetch|apiUUID:%s\n", apiUUID)

	conf, errReadConfig := config.ReadConfigs()
	if errReadConfig != nil {
		logger.LoggerSynchronizer.Errorf("Error reading configs for Subscription fetch, Error: %v", errReadConfig)
	}

	subscriptions, errorMsg := sync.FetchSubscriptions(apiUUID)
	if subscriptions == nil {
		return nil, errorMsg
	}

	if len(subscriptions) == 0 && errorMsg != constants.EmptyString {
		logger.LoggerSynchronizer.Warnf("Error fetching subscriptions in retry attempt %d : %s", retryAttempt, errorMsg)
		go retrySubscriptionsData(apiUUID, conf, errorMsg)
		return nil, errorMsg
	}

	logger.LoggerSynchronizer.Debugf("Fetched subscriptions for API %s - Total fetched: %d",
		apiUUID, len(subscriptions))

	return subscriptions, ""
}

func retrySubscriptionsData(apiUUID string, conf *config.Config, errorMessage string) {
	logger.LoggerSynchronizer.Debugf("Time Duration for retrying: %v",
		conf.ControlPlane.RetryInterval*time.Second)
	time.Sleep(conf.ControlPlane.RetryInterval * time.Second)
	FetchSubscriptions(apiUUID)
	retryAttempt++
	if retryAttempt > constants.MaxRetries {
		logger.LoggerSynchronizer.Error(errorMessage)
		return
	}
}

// CreateApplicationConsumerForBothEnvironments creates consumers for both production and sandbox environments
func CreateApplicationConsumerForBothEnvironments(applicationUUID string, c client.Client, conf *config.Config) {
	CreateApplicationConsumer(applicationUUID, c, conf, constants.EnvironmentProduction)
	CreateApplicationConsumer(applicationUUID, c, conf, constants.EnvironmentSandbox)
}

func CreateApplicationConsumer(applicationUUID string, c client.Client, conf *config.Config, environment string) {
	logger.LoggerSynchronizer.Debugf("Creating application consumer for ApplicationUUID: %s, Environment: %s", applicationUUID, environment)

	consumer := transformer.CreateConsumer(applicationUUID, environment, conf)
	consumer.Namespace = conf.DataPlane.Namespace

	internalk8sClient.DeployKongConsumerCR(consumer, c)
}

// ProcessApplicationRegistration handles issuer secrets, credentials, and consumer updates
func ProcessApplicationRegistration(applicationUUID, consumerKey, keyManagerName, tenantOrg, environment string, c client.Client, conf *config.Config) {
	logger.LoggerSynchronizer.Debugf("Received Application Registration Event: applicationUUID=%s, consumerKey=%s, environment=%s, keyManagerName=%s, tenantOrg=%s",
		applicationUUID, consumerKey, environment, keyManagerName, tenantOrg)

	issuerSecrets := internalk8sClient.GetK8sSecrets(
		map[string]string{
			constants.TypeLabel:           constants.IssuerSecretType,
			constants.OrganizationLabel:   transformer.GenerateSHA1Hash(tenantOrg),
			constants.KeyManagerNameLabel: transformer.PrepareDashedName(keyManagerName),
		},
		c, conf,
	)
	if len(issuerSecrets) == 0 {
		logger.LoggerSynchronizer.Errorf("No issuers are found")
		return
	}

	addCredentials := make([]string, 0, len(issuerSecrets))
	for _, issuerSecret := range issuerSecrets {
		jwtCredentialSecret := transformer.CreateIssuerKongSecretCredential(
			issuerSecret, conf,
			applicationUUID,
			consumerKey,
			environment,
		)
		internalk8sClient.DeploySecretCR(jwtCredentialSecret, c)
		addCredentials = append(addCredentials, jwtCredentialSecret.ObjectMeta.Name)
	}

	utils.RetryKongCRUpdate(func() error {
		internalk8sClient.UpdateKongConsumerCredential(
			applicationUUID,
			strings.ToLower(environment),
			c, conf,
			addCredentials,
			nil,
		)
		return nil
	}, constants.AddApplicationKeyTaskName, constants.MaxRetries)
}

func CreateSubscription(applicationUUID, apiUUID, policyID, tenantDomain string, aclGroupNames []string, c client.Client, conf *config.Config, environment string, isBlocked bool) {
	logger.LoggerSynchronizer.Debugf("Creating subscriptions for api:%s of application:%s in environment:%s\n", apiUUID, applicationUUID, environment)

	addCredentials := []string{}
	addAnnotations := []string{}

	if !isBlocked {
		aclCredentialSecretConfig := map[string]string{
			constants.GroupField: aclGroupNames[0],
		}
		aclCredentialSecret := transformer.GenerateK8sCredentialSecret(applicationUUID, apiUUID+environment, constants.ACLCredentialType, aclCredentialSecretConfig)
		aclCredentialSecret.Labels[constants.EnvironmentLabel] = strings.ToLower(environment)
		aclCredentialSecret.Namespace = conf.DataPlane.Namespace
		addCredentials = append(addCredentials, aclCredentialSecret.ObjectMeta.Name)
		internalk8sClient.DeploySecretCR(aclCredentialSecret, c)
	}

	if len(addCredentials) > 0 {
		updateErr := utils.RetryKongCRUpdate(func() error {
			return internalk8sClient.UpdateKongConsumerCredential(applicationUUID, environment, c, conf, addCredentials, nil)
		}, constants.UpdateConsumerCredentialTask, constants.MaxRetries)
		if updateErr != nil {
			logger.LoggerSynchronizer.Errorf("Failed to update Kong consumer credentials: %v", updateErr)
			return
		}
	}

	// update consumer subscription limit plugin annotation
	subscriptionPolicy := managementserver.GetSubscriptionPolicy(policyID, tenantDomain)

	if subscriptionPolicy.Name != constants.EmptyString && subscriptionPolicy.Name != constants.UnlimitedPolicyName {
		rateLimitCRName := transformer.GeneratePolicyCRName(subscriptionPolicy.Name, subscriptionPolicy.TenantDomain, constants.RateLimitingPlugin, constants.SubscriptionTypeKey)
		addAnnotations = append(addAnnotations, rateLimitCRName)
	}

	if len(addAnnotations) > 0 {
		updateErr := utils.RetryKongCRUpdate(func() error {
			return internalk8sClient.UpdateKongConsumerPluginAnnotation(applicationUUID, environment, c, conf, addAnnotations, nil)
		}, constants.UpdateConsumerPluginAnnotationTask, constants.MaxRetries)
		if updateErr != nil {
			logger.LoggerSynchronizer.Errorf("Failed to update Kong consumer plugin annotations: %v", updateErr)
			return
		}
	}
}
