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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	pkgAuth "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/auth"
	eventhub "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/eventhub/types"
	logger "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/loggers"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/tlsutils"
)

const (
	applicationsEndpoint string = "internal/data/v1/application-key-mappings"
)

// FetchApplicationKeyMappings fetches the policies from the control plane on the start up and notification event updates
func FetchApplicationKeyMappings(organization string) ([]eventhub.ApplicationKeyMapping, string) {
	logger.LoggerSync.Infof("Starting Application Key Mappings fetch Organization: %s", organization)

	// Read configurations and derive the eventHub details
	conf, errReadConfig := config.ReadConfigs()
	if errReadConfig != nil {
		// This has to be error. For debugging purpose info
		logger.LoggerSync.Errorf("Error reading configs for Application Key Mappings fetch Organization: %s, Error: %v", organization, errReadConfig)
	}

	// Populate data from the config
	ehConfigs := conf.ControlPlane
	ehURL := ehConfigs.ServiceURL

	// If the eventHub URL is configured with trailing slash
	if strings.HasSuffix(ehURL, "/") {
		ehURL += applicationsEndpoint
	} else {
		ehURL += "/" + applicationsEndpoint
	}

	logger.LoggerSync.Debugf("Complete endpoint URL constructed Organization: %s, Final URL: %s", organization, ehURL)

	ehUname := ehConfigs.Username
	ehPass := ehConfigs.Password
	basicAuth := "Basic " + pkgAuth.GetBasicAuth(ehUname, ehPass)

	// Check if TLS is enabled
	skipSSL := ehConfigs.SkipSSLVerification

	// Create a HTTP request
	req, err := http.NewRequest("GET", ehURL, nil)
	if err != nil {
		logger.LoggerSync.Errorf("HTTP request creation failed Organization: %s, URL: %s, Error: %v", organization, ehURL, err)
	}

	queryParamMap := make(map[string]string)

	if len(queryParamMap) > 0 {
		q := req.URL.Query()
		// Making necessary query parameters for the request
		for queryParamKey, queryParamValue := range queryParamMap {
			q.Add(queryParamKey, queryParamValue)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Setting authorization header
	req.Header.Set(Authorization, basicAuth)

	if organization != "" {
		req.Header.Set("xWSO2Tenant", organization)
	}

	// Make the request
	logger.LoggerSync.Debugf("Sending control plane request - URL: %s, Organization: %s, SkipSSL: %v", ehURL, organization, skipSSL)
	resp, err := tlsutils.InvokeControlPlane(req, skipSSL)
	var errorMsg string
	if err != nil {
		errorMsg = "Error occurred while calling the REST API: " + applicationsEndpoint
		logger.LoggerSync.Errorf("Control plane request failed - URL: %s, Organization: %s, Error: %v", ehURL, organization, err)
		return make([]eventhub.ApplicationKeyMapping, 0), errorMsg
	}

	responseBytes, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		errorMsg = "Error occurred while reading the response received for: " + applicationsEndpoint
		logger.LoggerSync.Errorf("Response body read failed - URL: %s, Organization: %s, Error: %v", ehURL, organization, err)
		return make([]eventhub.ApplicationKeyMapping, 0), errorMsg
	}

	if resp.StatusCode == http.StatusOK {
		var applicationKeyMappingList eventhub.ApplicationKeyMappingList
		err := json.Unmarshal(responseBytes, &applicationKeyMappingList)
		if err != nil {
			errorMsg = "Error occurred while JSON unmarshaling the response received for: " + applicationsEndpoint
			logger.LoggerSync.Errorf("JSON unmarshaling failed - URL: %s, Organization: %s, Response: %s, Error: %v", ehURL, organization, string(responseBytes), err)
			return make([]eventhub.ApplicationKeyMapping, 0), errorMsg
		}
		logger.LoggerSync.Debugf("Application Key Mappings successfully parsed - URL: %s, Organization: %s, Total Mappings: %d, Mappings: %+v",
			ehURL, organization, len(applicationKeyMappingList.List), applicationKeyMappingList.List)
		return applicationKeyMappingList.List, ""
	}

	errorMsg = "Failed to fetch data! " + applicationsEndpoint + " responded with " + strconv.Itoa(resp.StatusCode)
	logger.LoggerSync.Errorf("Control plane request failed with non-200 status - URL: %s, Organization: %s, Status Code: %d, Response Body: %s",
		ehURL, organization, resp.StatusCode, string(responseBytes))
	return make([]eventhub.ApplicationKeyMapping, 0), errorMsg
}
