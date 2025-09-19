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

package managementserver

import "sync"

var (
	processedAPIUUIDs map[string]struct{} // Hash set for processed API UUIDs
	processedAppUUIDs map[string]struct{} // Hash set for processed Application UUIDs
	apiMutex          sync.RWMutex        // Mutex for API UUID operations
	appMutex          sync.RWMutex        // Mutex for Application UUID operations
)

func init() {
	processedAPIUUIDs = make(map[string]struct{})
	processedAppUUIDs = make(map[string]struct{})
}

// AddProcessedAPI marks an API UUID as processed
func AddProcessedAPI(apiUUID string) {
	apiMutex.Lock()
	defer apiMutex.Unlock()
	processedAPIUUIDs[apiUUID] = struct{}{}
}

// IsAPIProcessed checks if an API UUID has been processed
func IsAPIProcessed(apiUUID string) bool {
	apiMutex.RLock()
	defer apiMutex.RUnlock()
	_, exists := processedAPIUUIDs[apiUUID]
	return exists
}

// RemoveProcessedAPI removes an API UUID from the processed list
func RemoveProcessedAPI(apiUUID string) {
	apiMutex.Lock()
	defer apiMutex.Unlock()
	if _, exists := processedAPIUUIDs[apiUUID]; exists {
		delete(processedAPIUUIDs, apiUUID)
	}
}

// GetAllProcessedAPIs returns all processed API UUIDs
func GetAllProcessedAPIs() []string {
	apiMutex.RLock()
	defer apiMutex.RUnlock()
	apiUUIDs := make([]string, 0, len(processedAPIUUIDs))
	for uuid := range processedAPIUUIDs {
		apiUUIDs = append(apiUUIDs, uuid)
	}
	return apiUUIDs
}

// AddProcessedApplication marks an Application UUID as processed
func AddProcessedApplication(appUUID string) {
	appMutex.Lock()
	defer appMutex.Unlock()
	processedAppUUIDs[appUUID] = struct{}{}
}

// IsApplicationProcessed checks if an Application UUID has been processed
func IsApplicationProcessed(appUUID string) bool {
	appMutex.RLock()
	defer appMutex.RUnlock()
	_, exists := processedAppUUIDs[appUUID]
	return exists
}

// RemoveProcessedApplication removes an Application UUID from the processed list
func RemoveProcessedApplication(appUUID string) {
	appMutex.Lock()
	defer appMutex.Unlock()
	if _, exists := processedAppUUIDs[appUUID]; exists {
		delete(processedAppUUIDs, appUUID)
	}
}

// GetAllProcessedApplications returns all processed Application UUIDs
func GetAllProcessedApplications() []string {
	appMutex.RLock()
	defer appMutex.RUnlock()
	appUUIDs := make([]string, 0, len(processedAppUUIDs))
	for uuid := range processedAppUUIDs {
		appUUIDs = append(appUUIDs, uuid)
	}
	return appUUIDs
}
