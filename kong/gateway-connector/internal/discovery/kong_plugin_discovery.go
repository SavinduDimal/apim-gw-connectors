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

package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	discoverPkg "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/discovery"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/managementserver"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/constants"
	"github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector/internal/loggers"
)

// handleUpdateKongPluginResource handles the update of an KongPlugin
func handleUpdateKongPluginResource(oldKongPlugin, kongPlugin *unstructured.Unstructured) {
	loggers.LoggerWatcher.Infof("Processing KongPlugin modification: %s/%s (Generation: %d, ResourceVersion: %s)",
		kongPlugin.GetNamespace(), kongPlugin.GetName(), kongPlugin.GetGeneration(), kongPlugin.GetResourceVersion())

	httpRouteList, err := CRWatcher.DynamicClient.Resource(constants.HTTPRouteGVR).Namespace(kongPlugin.GetNamespace()).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: constants.K8sInitiatedFromField + constants.EqualString + constants.DataPlaneOrigin,
		},
	)
	if err != nil {
		loggers.LoggerWatcher.Errorf("Failed to list HTTPRoutes for KongPlugin %s/%s: %v", kongPlugin.GetNamespace(), kongPlugin.GetName(), err)
		return
	}

	var matchingHTTPRoutes []*unstructured.Unstructured
	kongPluginName := kongPlugin.GetName()

	for i := range httpRouteList.Items {
		httpRoute := &httpRouteList.Items[i]

		if annotations := httpRoute.GetAnnotations(); annotations != nil {
			if pluginsAnnotation, exists := annotations[constants.KongPluginsAnnotation]; exists {
				pluginNames := strings.Split(pluginsAnnotation, constants.CommaString)
				for _, pluginName := range pluginNames {
					pluginName = strings.TrimSpace(pluginName)
					if pluginName == kongPluginName {
						matchingHTTPRoutes = append(matchingHTTPRoutes, httpRoute)
						loggers.LoggerWatcher.Debugf("Found HTTPRoute %s/%s using KongPlugin %s",
							httpRoute.GetNamespace(), httpRoute.GetName(), kongPluginName)
						break
					}
				}
			}
		}
	}

	if len(matchingHTTPRoutes) == 0 {
		loggers.LoggerWatcher.Infof("No HTTPRoutes found using KongPlugin %s/%s, skipping processing",
			kongPlugin.GetNamespace(), kongPluginName)
		return
	}

	loggers.LoggerWatcher.Infof("Found %d HTTPRoutes using KongPlugin %s/%s",
		len(matchingHTTPRoutes), kongPlugin.GetNamespace(), kongPluginName)

	// Collect all unique APIs that use this KongPlugin
	uniqueAPIs := make(map[string]managementserver.API)
	var apiUUIDs []string

	for _, route := range matchingHTTPRoutes {
		if kongAPIUUID, hasUUID := route.GetLabels()[constants.KongAPIUUIDLabel]; hasUUID {
			if apiFromMap, exists := discoverPkg.APIMap[kongAPIUUID]; exists {
				if _, alreadyFound := uniqueAPIs[kongAPIUUID]; !alreadyFound {
					uniqueAPIs[kongAPIUUID] = apiFromMap
					apiUUIDs = append(apiUUIDs, kongAPIUUID)
					loggers.LoggerWatcher.Debugf("Found API %s for HTTPRoute %s/%s", kongAPIUUID, route.GetNamespace(), route.GetName())
				}
			}
		}
	}

	if len(uniqueAPIs) == 0 {
		loggers.LoggerWatcher.Warnf("No APIs found for HTTPRoutes using KongPlugin %s/%s", kongPlugin.GetNamespace(), kongPluginName)
		return
	}

	loggers.LoggerWatcher.Infof("Found %d unique APIs using KongPlugin %s/%s: %v",
		len(uniqueAPIs), kongPlugin.GetNamespace(), kongPluginName, apiUUIDs)

	pluginType, found, _ := unstructured.NestedString(kongPlugin.Object, constants.PluginField)
	if !found {
		loggers.LoggerWatcher.Warnf("KongPlugin %s has no plugin field", kongPlugin.GetName())
		return
	}
	switch pluginType {
	case constants.CORSPlugin:
		handleCORSPluginUpdate(kongPlugin, uniqueAPIs, kongPluginName)
	case constants.JWTPlugin:
		handleJWTPluginUpdate(kongPlugin, uniqueAPIs, kongPluginName)
	case constants.ACLPlugin:
		handleACLPluginUpdate(oldKongPlugin, kongPlugin, uniqueAPIs, kongPluginName)
	}
}

// handleCORSPluginUpdate handles CORS plugin updates
func handleCORSPluginUpdate(kongPlugin *unstructured.Unstructured, uniqueAPIs map[string]managementserver.API, kongPluginName string) {
	loggers.LoggerWatcher.Debugf("Processing CORS plugin update for KongPlugin %s", kongPluginName)

	newCORSPolicy := extractCORSPolicyFromKongPlugin(kongPlugin)
	newHash := constants.EmptyString
	if newCORSPolicy != nil {
		newHash = computeCORSPolicyHash(newCORSPolicy)
	}

	var updatedAPIs []string
	for kongAPIUUID, api := range uniqueAPIs {
		existingCORSPolicy := api.CORSPolicy
		existingHash := constants.EmptyString
		if existingCORSPolicy != nil {
			existingHash = computeCORSPolicyHash(existingCORSPolicy)
		}

		if existingHash == newHash && existingHash != constants.EmptyString {
			loggers.LoggerWatcher.Debugf("CORS policy hash unchanged for API %s in KongPlugin %s (hash: %s), skipping update",
				kongAPIUUID, kongPluginName, existingHash)
			continue
		}

		loggers.LoggerWatcher.Debugf("CORS policy changed for API %s in KongPlugin %s - old hash: %s, new hash: %s",
			kongAPIUUID, kongPluginName, existingHash, newHash)

		if newCORSPolicy != nil {
			api.CORSPolicy = newCORSPolicy

			discoverPkg.APIHashMap[kongAPIUUID] = computeAPIHash(api)
			discoverPkg.APIMap[kongAPIUUID] = api
			discoverPkg.QueueEvent(managementserver.CreateEvent, api, api.APIName, kongPlugin.GetNamespace(), constants.DefaultKongAgentName, kongAPIUUID)
			updatedAPIs = append(updatedAPIs, kongAPIUUID)
			loggers.LoggerWatcher.Infof("Updated API %s with new CORS policy from KongPlugin %s (new hash: %s)",
				kongAPIUUID, kongPluginName, newHash)
		}
	}

	if len(updatedAPIs) > 0 {
		loggers.LoggerWatcher.Infof("CORS policy update completed for %d APIs", len(updatedAPIs))
	}
}

// handleJWTPluginUpdate handles JWT plugin updates
func handleJWTPluginUpdate(kongPlugin *unstructured.Unstructured, uniqueAPIs map[string]managementserver.API, kongPluginName string) {
	loggers.LoggerWatcher.Debugf("Processing JWT plugin update for KongPlugin %s", kongPluginName)

	newAuthHeader := extractAuthHeaderFromJWTPolicyKongPlugin(kongPlugin)
	if newAuthHeader != constants.EmptyString {
		var updatedAPIs []string
		for kongAPIUUID, api := range uniqueAPIs {
			if api.AuthHeader != newAuthHeader {
				api.AuthHeader = newAuthHeader

				discoverPkg.APIHashMap[kongAPIUUID] = computeAPIHash(api)
				discoverPkg.APIMap[kongAPIUUID] = api
				discoverPkg.QueueEvent(managementserver.CreateEvent, api, api.APIName, kongPlugin.GetNamespace(), constants.DefaultKongAgentName, kongAPIUUID)
				updatedAPIs = append(updatedAPIs, kongAPIUUID)
				loggers.LoggerWatcher.Debugf("Updated API %s with new auth header from KongPlugin %s", kongAPIUUID, kongPluginName)
			} else {
				loggers.LoggerWatcher.Debugf("Auth header unchanged for API %s in KongPlugin %s, skipping update", kongAPIUUID, kongPluginName)
			}
		}

		if len(updatedAPIs) > 0 {
			loggers.LoggerWatcher.Infof("JWT auth header update completed for %d APIs: %v", len(updatedAPIs), updatedAPIs)
		}
	}
}

// handleACLPluginUpdate handles ACL plugin updates
func handleACLPluginUpdate(oldKongPlugin, kongPlugin *unstructured.Unstructured, uniqueAPIs map[string]managementserver.API, kongPluginName string) {
	loggers.LoggerWatcher.Debugf("Processing ACL plugin update for KongPlugin %s", kongPluginName)

	newACLAllowValues, err := ExtractACLAllowValuesFromPlugin(kongPlugin)
	if err != nil {
		loggers.LoggerWatcher.Errorf("Failed to extract ACL allow values from KongPlugin %s: %v", kongPluginName, err)
		return
	}
	oldACLAllowValues, err := ExtractACLAllowValuesFromPlugin(oldKongPlugin)
	if err != nil {
		loggers.LoggerWatcher.Errorf("Failed to extract ACL allow values from old KongPlugin %s: %v", kongPluginName, err)
		return
	}

	// Compare old and new ACL allow values
	sameValues := false
	if len(oldACLAllowValues) == len(newACLAllowValues) {
		sameValues = true
		oldValuesMap := make(map[string]bool)
		for _, val := range oldACLAllowValues {
			oldValuesMap[val] = true
		}
		for _, val := range newACLAllowValues {
			if !oldValuesMap[val] {
				sameValues = false
				break
			}
		}
	}

	if sameValues {
		loggers.LoggerWatcher.Debugf("ACL allow values unchanged for KongPlugin %s, skipping update", kongPluginName)
		return
	}

	// Update ACL secrets with new group values
	if len(newACLAllowValues) > 0 && len(oldACLAllowValues) > 0 {
		loggers.LoggerWatcher.Infof("Updating ACL secrets: old values %v → new values %v", oldACLAllowValues, newACLAllowValues)
		err := updateACLSecretsWithNewGroup(oldACLAllowValues, newACLAllowValues[0], kongPlugin.GetNamespace())
		if err != nil {
			loggers.LoggerWatcher.Errorf("Failed to update ACL secrets for KongPlugin %s: %v", kongPluginName, err)
		}
	}
}

// updateACLSecretsWithNewGroup updates ACL secrets by finding secrets with old group values and updating them with new group value
func updateACLSecretsWithNewGroup(oldACLAllowValues []string, newGroupValue, namespace string) error {
	loggers.LoggerWatcher.Infof("Starting ACL secrets update")

	secretList, err := CRWatcher.DynamicClient.Resource(constants.SecretGVR).Namespace(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: constants.KongCredentialLabel + constants.EqualString + constants.ACLCredentialType,
	})
	if err != nil {
		return fmt.Errorf("failed to list ACL secrets in namespace %s: %v", namespace, err)
	}

	loggers.LoggerWatcher.Debugf("Found %d ACL secrets in namespace %s", len(secretList.Items), namespace)

	var updatedSecrets []string
	newGroupValueEncoded := base64.StdEncoding.EncodeToString([]byte(newGroupValue))

	for i := range secretList.Items {
		secret := &secretList.Items[i]
		secretName := secret.GetName()

		if dataMap, found, _ := unstructured.NestedMap(secret.Object, constants.DataField); found {
			if groupDataInterface, exists := dataMap[constants.GroupField]; exists {
				if groupDataStr, ok := groupDataInterface.(string); ok {
					currentGroupBytes, err := base64.StdEncoding.DecodeString(groupDataStr)
					if err != nil {
						loggers.LoggerWatcher.Warnf("Failed to decode group data for secret %s: %v", secretName, err)
						continue
					}
					currentGroup := string(currentGroupBytes)

					shouldUpdate := false
					for _, oldValue := range oldACLAllowValues {
						if currentGroup == oldValue {
							shouldUpdate = true
							loggers.LoggerWatcher.Debugf("Secret %s has matching old group value", secretName)
							break
						}
					}

					if shouldUpdate {
						secretCopy := secret.DeepCopy()
						err := unstructured.SetNestedField(secretCopy.Object, newGroupValueEncoded, constants.DataField, constants.GroupField)
						if err != nil {
							loggers.LoggerWatcher.Errorf("Failed to set group data for secret %s: %v", secretName, err)
							continue
						}

						_, err = CRWatcher.DynamicClient.Resource(constants.SecretGVR).Namespace(namespace).Update(context.Background(), secretCopy, metav1.UpdateOptions{})
						if err != nil {
							loggers.LoggerWatcher.Errorf("Failed to update secret %s with new group value: %v", secretName, err)
							continue
						}

						updatedSecrets = append(updatedSecrets, secretName)
						loggers.LoggerWatcher.Infof("Updated ACL secret %s", secretName)
					} else {
						loggers.LoggerWatcher.Debugf("Secret %s group value does not match any old values, skipping", secretName)
					}
				}
			}
		}
	}

	return nil
}

// computeCORSPolicyHash generates a hash of the CORSPolicy for comparison
func computeCORSPolicyHash(corsPolicy *managementserver.CORSPolicy) string {
	if corsPolicy == nil {
		return constants.EmptyString
	}

	normalizedPolicy := &managementserver.CORSPolicy{
		AccessControlAllowCredentials: corsPolicy.AccessControlAllowCredentials,
		AccessControlMaxAge:           corsPolicy.AccessControlMaxAge,
	}

	if corsPolicy.AccessControlAllowOrigins != nil {
		origins := make([]string, len(corsPolicy.AccessControlAllowOrigins))
		copy(origins, corsPolicy.AccessControlAllowOrigins)
		sort.Strings(origins)
		normalizedPolicy.AccessControlAllowOrigins = origins
	}

	if corsPolicy.AccessControlAllowHeaders != nil {
		headers := make([]string, len(corsPolicy.AccessControlAllowHeaders))
		copy(headers, corsPolicy.AccessControlAllowHeaders)
		sort.Strings(headers)
		normalizedPolicy.AccessControlAllowHeaders = headers
	}

	if corsPolicy.AccessControlAllowMethods != nil {
		methods := make([]string, len(corsPolicy.AccessControlAllowMethods))
		copy(methods, corsPolicy.AccessControlAllowMethods)
		sort.Strings(methods)
		normalizedPolicy.AccessControlAllowMethods = methods
	}

	if corsPolicy.AccessControlExposeHeaders != nil {
		exposeHeaders := make([]string, len(corsPolicy.AccessControlExposeHeaders))
		copy(exposeHeaders, corsPolicy.AccessControlExposeHeaders)
		sort.Strings(exposeHeaders)
		normalizedPolicy.AccessControlExposeHeaders = exposeHeaders
	}

	data := fmt.Sprintf("%v", normalizedPolicy)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ExtractACLAllowValuesFromPlugin extracts ACL allow values from a single Kong plugin
func ExtractACLAllowValuesFromPlugin(plugin *unstructured.Unstructured) ([]string, error) {
	if plugin == nil {
		return nil, nil
	}

	pluginType, found, err := unstructured.NestedString(plugin.Object, constants.PluginField)
	if !found || err != nil || pluginType != constants.ACLPlugin {
		return nil, nil
	}

	config, found, err := unstructured.NestedMap(plugin.Object, constants.ConfigField)
	if !found || err != nil {
		return nil, nil
	}

	allow, found, err := unstructured.NestedSlice(config, constants.AllowField)
	if !found || err != nil {
		return nil, nil
	}

	var allowValues []string
	for _, val := range allow {
		if strVal, ok := val.(string); ok && strVal != constants.EmptyString {
			allowValues = append(allowValues, strVal)
		}
	}

	return allowValues, nil
}
