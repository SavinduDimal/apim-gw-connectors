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
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	v1 "github.com/kong/kubernetes-configuration/api/configuration/v1"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	eventConstants "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/constants"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/k8s-resource-lib/constants"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/managementserver"
	msg "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/messaging"
	kongConstants "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/discovery"
	internalk8sClient "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/k8sClient"
	logger "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/loggers"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/utils"
	kongMgtServer "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/managementserver"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/synchronizer"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/pkg/transformer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleSubscriptionEvents to process subscription related events
func HandleSubscriptionEvents(data []byte, eventType string, c client.Client) {
	logger.LoggerEvents.Infof("Processing subscription event processing with EventType: %s, data length: %d bytes", eventType, len(data))

	conf, errReadConfig := config.ReadConfigs()
	if errReadConfig != nil {
		logger.LoggerEvents.Errorf("Error reading configs: %v", errReadConfig)
		return
	}

	var subscriptionEvent msg.SubscriptionEvent
	if subEventErr := json.Unmarshal(data, &subscriptionEvent); subEventErr != nil {
		logger.LoggerEvents.Errorf("%s: %v", kongConstants.UnmarshalErrorSubscription, subEventErr)
		return
	}

	if !belongsToTenant(subscriptionEvent.TenantDomain) {
		logger.LoggerEvents.Debugf("Subscription event for the Application: %s and API %s is dropped due to having non related tenantDomain: %s",
			subscriptionEvent.ApplicationUUID, subscriptionEvent.APIUUID, subscriptionEvent.TenantDomain)
		return
	}

	if isLaterEvent(subsriptionsListTimeStampMap, fmt.Sprint(subscriptionEvent.SubscriptionID), subscriptionEvent.TimeStamp) {
		return
	}

	if !kongMgtServer.IsAPIProcessed(subscriptionEvent.APIUUID) {
		logger.LoggerEvents.Infof("API %s is not processed. skipping subscription event", subscriptionEvent.APIUUID)
		return
	}

	logger.LoggerEvents.Debugf("Received Subscription Event: %+v", subscriptionEvent)
	switch subscriptionEvent.Event.Type {
	case eventConstants.SubscriptionCreate:
		if !kongMgtServer.IsApplicationProcessed(subscriptionEvent.ApplicationUUID) {
			kongMgtServer.AddProcessedApplication(subscriptionEvent.ApplicationUUID)
			synchronizer.CreateApplicationConsumerForBothEnvironments(subscriptionEvent.ApplicationUUID, c, conf)
		}
		createSecretsForKeyGeneration(subscriptionEvent, c, conf)
		createSubscription(subscriptionEvent, c, conf, constants.ProductionType)
		createSubscription(subscriptionEvent, c, conf, constants.SandboxType)
	case eventConstants.SubscriptionUpdate:
		updateSubscription(subscriptionEvent, c, conf, constants.ProductionType)
		updateSubscription(subscriptionEvent, c, conf, constants.SandboxType)
	case eventConstants.SubscriptionDelete:
		removeSubscription(subscriptionEvent, c, conf, constants.ProductionType)
		removeSubscription(subscriptionEvent, c, conf, constants.SandboxType)
	}
}

func createSubscription(subscriptionEvent msg.SubscriptionEvent, c client.Client, conf *config.Config, environment string) {
	logger.LoggerEvents.Debugf("Creating subscription|ApplicationUUID:%s Environment:%s\n", subscriptionEvent.ApplicationUUID, environment)

	aclGroupNames, err := generateACLGroupNameFromK8s(subscriptionEvent.APIName, environment, c, conf)
	if err != nil {
		logger.LoggerEvents.Debugf("Failed to generate ACL group names from K8s resources: %v", err)
		logger.LoggerEvents.Infof("Generating the ACL group name using general method")
		aclGroupNames = []string{transformer.GenerateACLGroupName(subscriptionEvent.APIName, environment)}
	}

	synchronizer.CreateSubscription(subscriptionEvent.ApplicationUUID, subscriptionEvent.APIUUID, subscriptionEvent.PolicyID,
		subscriptionEvent.TenantDomain, aclGroupNames, c, conf, environment, false)
}

func updateSubscription(subscriptionEvent msg.SubscriptionEvent, c client.Client, conf *config.Config, environment string) {
	logger.LoggerEvents.Debugf("Updating subscription|ApplicationUUID:%s Environment:%s\n", subscriptionEvent.ApplicationUUID, environment)

	var removeAnnotations []string
	var addAnnotations []string
	// retrieving current production subscription policy
	consumerName := transformer.GenerateConsumerName(subscriptionEvent.ApplicationUUID, environment)
	consumer := internalk8sClient.GetKongConsumerCR(consumerName, c, conf)

	if consumer == nil {
		logger.LoggerEvents.Infof("Kong consumer credential not found for %v", environment)
	} else {
		subscriptionPolicy := managementserver.GetSubscriptionPolicy(subscriptionEvent.PolicyID, subscriptionEvent.TenantDomain)
		rateLimitCRName := transformer.GeneratePolicyCRName(subscriptionPolicy.Name, subscriptionPolicy.TenantDomain, kongConstants.RateLimitingPlugin, kongConstants.SubscriptionTypeKey)
		// handle subscription rate limiting
		if annotations, ok := consumer.Annotations[kongConstants.KongPluginsAnnotation]; ok {
			annotationsArr := strings.Split(annotations, kongConstants.CommaString)
			if !slices.Contains(annotationsArr, rateLimitCRName) {
				// remove old subscription policy name
				for _, name := range annotationsArr {
					if strings.Contains(name, kongConstants.SubscriptionTypeKey) && strings.Contains(name, kongConstants.RateLimitingPlugin) {
						removeAnnotations = append(removeAnnotations, name)
						break
					}
				}

				// updating new subscription policy name
				if subscriptionPolicy.Name != kongConstants.EmptyString && subscriptionPolicy.Name != kongConstants.UnlimitedPolicyName {
					addAnnotations = append(addAnnotations, rateLimitCRName)
				}

				err := utils.RetryKongCRUpdate(func() error {
					return internalk8sClient.UpdateKongConsumerPluginAnnotation(subscriptionEvent.ApplicationUUID, environment, c, conf, addAnnotations, removeAnnotations)
				}, kongConstants.UpdateConsumerPluginAnnotationTask, kongConstants.MaxRetries)
				if err != nil {
					logger.LoggerEvents.Errorf("Failed to update consumer plugin annotations: %v", err)
					return
				}
			}
		}

		// handle subscription state
		subscriptionIdentifier := subscriptionEvent.APIUUID + environment
		aclCredentialSecretName := transformer.GenerateSecretName(subscriptionEvent.ApplicationUUID, subscriptionIdentifier, kongConstants.ACLCredentialType)
		credentials := []string{aclCredentialSecretName}

		switch subscriptionEvent.SubscriptionState {
		case kongConstants.SubscriptionStateBlocked:
			if err := utils.RetryKongCRUpdate(func() error {
				return internalk8sClient.UpdateKongConsumerCredential(subscriptionEvent.ApplicationUUID, environment, c, conf, nil, credentials)
			}, kongConstants.UpdateConsumerCredentialBlockedTask+environment, kongConstants.MaxRetries); err != nil {
				logger.LoggerEvents.Errorf("Failed to block %s credentials: %v", environment, err)
				return
			}
		case kongConstants.SubscriptionStateProdOnlyBlocked:
			if environment == constants.ProductionType {
				if err := utils.RetryKongCRUpdate(func() error {
					return internalk8sClient.UpdateKongConsumerCredential(subscriptionEvent.ApplicationUUID, environment, c, conf, nil, credentials)
				}, kongConstants.UpdateConsumerCredentialProdBlockedTask+environment, kongConstants.MaxRetries); err != nil {
					logger.LoggerEvents.Errorf("Failed to enable %s credentials for prod-only-blocked: %v", environment, err)
					return
				}
			} else {
				if err := utils.RetryKongCRUpdate(func() error {
					return internalk8sClient.UpdateKongConsumerCredential(subscriptionEvent.ApplicationUUID, environment, c, conf, credentials, nil)
				}, kongConstants.UpdateConsumerCredentialProdBlockedTask+environment, kongConstants.MaxRetries); err != nil {
					logger.LoggerEvents.Errorf("Failed to enable %s credentials for prod-only-blocked: %v", environment, err)
					return
				}

			}
		case kongConstants.SubscriptionStateUnblocked:
			if err := utils.RetryKongCRUpdate(func() error {
				return internalk8sClient.UpdateKongConsumerCredential(subscriptionEvent.ApplicationUUID, environment, c, conf, credentials, nil)
			}, kongConstants.UpdateConsumerCredentialUnblockedTask+environment, kongConstants.MaxRetries); err != nil {
				logger.LoggerEvents.Errorf("Failed to unblock %s credentials: %v", environment, err)
				return
			}
		}
	}

}

func removeSubscription(subscriptionEvent msg.SubscriptionEvent, c client.Client, conf *config.Config, environment string) {
	logger.LoggerEvents.Debugf("Removing subscription | ApplicationUUID:%s | Environment:%s", subscriptionEvent.ApplicationUUID, environment)

	appUUID := subscriptionEvent.ApplicationUUID
	apiUUID := subscriptionEvent.APIUUID
	subscriptionIdentifier := apiUUID + environment

	aclSecretCredentialName := transformer.GenerateSecretName(appUUID, subscriptionIdentifier, kongConstants.ACLCredentialType)
	consumerName := transformer.GenerateConsumerName(appUUID, environment)

	// Remove secret from Kong consumer CR
	if err := utils.RetryKongCRUpdate(func() error {
		return internalk8sClient.UpdateKongConsumerCredential(appUUID, environment, c, conf, nil, []string{aclSecretCredentialName})
	}, kongConstants.UpdateConsumerCredentialRemoveTask, kongConstants.MaxRetries); err != nil {
		logger.LoggerEvents.Errorf("Failed to remove Kong consumer credential secret (%s): %v", aclSecretCredentialName, err)
		return
	}

	// Undeploy ACL secret credential
	internalk8sClient.UnDeploySecretCR(aclSecretCredentialName, c, conf)

	// Get the consumer CR
	consumer := internalk8sClient.GetKongConsumerCR(consumerName, c, conf)
	if consumer == nil {
		logger.LoggerEvents.Infof("Kong consumer CR not found for environment %s", environment)
		return
	}

	// Handle consumer plugin annotations
	handleConsumerAnnotations(subscriptionEvent, consumer, c, conf, environment)

	// Handle consumer credentials cleanup
	handleConsumerCredentials(consumer, consumerName, appUUID, c, conf)
}

func handleConsumerAnnotations(subscriptionEvent msg.SubscriptionEvent, consumer *v1.KongConsumer, c client.Client, conf *config.Config, environment string) {
	annotations, ok := consumer.Annotations[kongConstants.KongPluginsAnnotation]
	if !ok || annotations == "" {
		return
	}

	subscriptionPolicy := managementserver.GetSubscriptionPolicy(subscriptionEvent.PolicyID, subscriptionEvent.TenantDomain)
	if subscriptionPolicy.Name == kongConstants.EmptyString {
		logger.LoggerEvents.Warnf("Subscription policy not found for PolicyID:%s Tenant:%s", subscriptionEvent.PolicyID, subscriptionEvent.TenantDomain)
		return
	}

	rateLimitCRName := transformer.GeneratePolicyCRName(subscriptionPolicy.Name, subscriptionPolicy.TenantDomain,
		kongConstants.RateLimitingPlugin, kongConstants.SubscriptionTypeKey)

	annotationsArr := strings.Split(annotations, kongConstants.CommaString)
	if !slices.Contains(annotationsArr, rateLimitCRName) {
		return
	}

	removeAnnotations := []string{rateLimitCRName}

	if err := utils.RetryKongCRUpdate(func() error {
		return internalk8sClient.UpdateKongConsumerPluginAnnotation(subscriptionEvent.ApplicationUUID, environment, c, conf, nil, removeAnnotations)
	}, kongConstants.UpdateConsumerPluginAnnotationTask, kongConstants.MaxRetries); err != nil {
		logger.LoggerEvents.Errorf("Failed to remove rate-limit annotation (%s) for consumer %s: %v",
			rateLimitCRName, consumer.Name, err)
	}
}

func handleConsumerCredentials(consumer *v1.KongConsumer, consumerName string, applicationUUID string, c client.Client, conf *config.Config) {
	credentials := consumer.Credentials
	if len(credentials) == 0 {
		logger.LoggerEvents.Debugf("No credentials found for consumer %s, deleting consumer", consumerName)
		internalk8sClient.UnDeployKongConsumerCR(consumerName, c, conf)
		kongMgtServer.RemoveProcessedApplication(applicationUUID)
		return
	}

	hasACLCredential := false
	for _, credential := range credentials {
		if strings.Contains(credential, kongConstants.SecretPrefix) && strings.Contains(credential, kongConstants.ACLCredentialType) {
			hasACLCredential = true
			break
		}
	}

	if !hasACLCredential {
		logger.LoggerEvents.Debugf("No ACL credentials for consumer %s, undeploying all credential secrets and deleting consumer", consumerName)
		for _, credential := range credentials {
			if strings.Contains(credential, kongConstants.SecretPrefix) {
				logger.LoggerEvents.Debugf("Undeploying credential secret: %s", credential)
				internalk8sClient.UnDeploySecretCR(credential, c, conf)
			}
		}
		internalk8sClient.UnDeployKongConsumerCR(consumerName, c, conf)
		kongMgtServer.RemoveProcessedApplication(applicationUUID)
		return
	}

	logger.LoggerEvents.Debugf("ACL credentials found for consumer %s, keeping consumer", consumerName)
}

// createSecretsForKeyGeneration creates secrets for both production and sandbox environments
func createSecretsForKeyGeneration(subscriptionEvent msg.SubscriptionEvent, c client.Client, conf *config.Config) {
	logger.LoggerEvents.Debugf("Creating secrets for application key mappings for ApplicationUUID: %s, Environment: %s",
		subscriptionEvent.ApplicationUUID, subscriptionEvent.TenantDomain)

	applicationKeyMappings, _ := synchronizer.FetchApplicationKeyMappingsOnEvent(subscriptionEvent.ApplicationUUID, subscriptionEvent.TenantDomain, c)

	if len(applicationKeyMappings) > 0 {
		logger.LoggerEvents.Debugf("Fetched %d application key mappings for application %s", len(applicationKeyMappings), subscriptionEvent.ApplicationUUID)
		for _, applicationKeyMapping := range applicationKeyMappings {
			synchronizer.ProcessApplicationRegistration(
				applicationKeyMapping.ApplicationUUID,
				applicationKeyMapping.ConsumerKey,
				applicationKeyMapping.KeyManager,
				subscriptionEvent.TenantDomain,
				strings.ToLower(applicationKeyMapping.KeyType),
				c,
				conf,
			)
		}
	}
}

// generateACLGroupNameFromK8s generates ACL group names by discovering services and their ACL plugins
func generateACLGroupNameFromK8s(apiName, environment string, c client.Client, conf *config.Config) ([]string, error) {
	service, err := findServiceByAPIName(apiName, c, conf)
	if err != nil || service == nil {
		return nil, fmt.Errorf("failed to find service for API %s: %v", apiName, err)
	}

	httpRoutes, err := getHTTPRoutesForService(service.Name, service.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTPRoutes for service %s: %v", service.Name, err)
	}

	allPluginNames := findAllPluginsFromHTTPRoutes(httpRoutes)
	logger.LoggerEvents.Debugf("Found %d plugin names from HTTPRoutes for service %s: %v",
		len(allPluginNames), service.Name, allPluginNames)

	allowValues, err := extractACLAllowValues(allPluginNames, service.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to extract ACL allow values: %v", err)
	}

	logger.LoggerEvents.Debugf("Extracted %d ACL allow values for service %s",
		len(allowValues), service.Name)

	if len(allowValues) > 0 {
		return allowValues, nil
	}

	fallbackGroup := transformer.GenerateACLGroupName(apiName, environment)
	return []string{fallbackGroup}, nil
}

// findServiceByAPIName finds a service by API name in all namespaces
func findServiceByAPIName(apiName string, c client.Client, conf *config.Config) (*corev1.Service, error) {
	if apiName == kongConstants.EmptyString {
		return nil, fmt.Errorf("API name cannot be empty")
	}

	namespace := conf.DataPlane.Namespace
	serviceList := &corev1.ServiceList{}
	ctx := context.Background()

	err := c.List(ctx, serviceList, client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	for _, service := range serviceList.Items {
		if service.Name == apiName {
			return &service, nil
		}
	}

	return nil, fmt.Errorf("service not found for API name: %s", apiName)
}

// getHTTPRoutesForService finds HTTPRoutes that reference the given service
func getHTTPRoutesForService(serviceName, namespace string) ([]unstructured.Unstructured, error) {
	httpRoutes := discovery.FetchAllHTTPRoutesWithServiceName(namespace, serviceName)

	if len(httpRoutes) == 0 {
		return nil, nil
	}

	result := make([]unstructured.Unstructured, 0, len(httpRoutes))
	for _, route := range httpRoutes {
		if route != nil {
			result = append(result, *route)
		}
	}

	return result, nil
}

// findAllPluginsFromHTTPRoutes extracts all plugin names from HTTPRoute annotations
func findAllPluginsFromHTTPRoutes(httpRoutes []unstructured.Unstructured) []string {
	if len(httpRoutes) == 0 {
		return nil
	}

	var allPluginNames []string
	pluginNameSet := make(map[string]bool)

	for _, route := range httpRoutes {
		annotations, found, err := unstructured.NestedStringMap(route.Object, kongConstants.MetadataField, kongConstants.AnnotationsField)
		if !found || err != nil {
			continue
		}
		for key, value := range annotations {
			if strings.Contains(key, kongConstants.KongPluginsAnnotation) {
				pluginNames := strings.Split(value, kongConstants.CommaString)
				for _, pluginName := range pluginNames {
					pluginName = strings.TrimSpace(pluginName)
					if pluginName != kongConstants.EmptyString && !pluginNameSet[pluginName] {
						pluginNameSet[pluginName] = true
						allPluginNames = append(allPluginNames, pluginName)
					}
				}
			}
		}
	}

	return allPluginNames
}

// extractACLAllowValues gets ACL plugin resources and extracts config.allow values
func extractACLAllowValues(allPluginNames []string, namespace string) ([]string, error) {
	if len(allPluginNames) == 0 {
		return nil, nil
	}

	var allowValues []string
	allowValueSet := make(map[string]bool)

	for _, pluginName := range allPluginNames {
		plugin := discovery.FetchKongPlugin(namespace, pluginName)
		if plugin == nil {
			logger.LoggerEvents.Warnf("Failed to get KongPlugin %s", pluginName)
			continue
		}

		pluginType, found, err := unstructured.NestedString(plugin.Object, kongConstants.PluginField)
		if !found || err != nil || pluginType != kongConstants.ACLPlugin {
			continue
		}

		config, found, err := unstructured.NestedMap(plugin.Object, kongConstants.ConfigField)
		if !found || err != nil {
			continue
		}

		allow, found, err := unstructured.NestedSlice(config, kongConstants.AllowField)
		if found && err == nil {
			for _, val := range allow {
				if strVal, ok := val.(string); ok && strVal != kongConstants.EmptyString {
					if !allowValueSet[strVal] {
						allowValueSet[strVal] = true
						allowValues = append(allowValues, strVal)
					}
				}
			}
		}
	}

	return allowValues, nil
}
