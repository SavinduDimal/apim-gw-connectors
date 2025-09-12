/*
 * Copyright (c) 2025 WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package org.wso2.kong.client;

import feign.Feign;
import feign.Request;
import feign.RequestInterceptor;
import feign.gson.GsonDecoder;
import feign.gson.GsonEncoder;
import feign.slf4j.Slf4jLogger;

import org.apache.commons.lang3.StringUtils;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.apache.http.client.HttpClient;

import org.wso2.carbon.apimgt.api.APIManagementException;
import org.wso2.carbon.apimgt.api.model.API;
import org.wso2.carbon.apimgt.api.model.Environment;
import org.wso2.carbon.apimgt.api.model.GatewayAPIValidationResult;
import org.wso2.carbon.apimgt.api.model.GatewayDeployer;
import org.wso2.carbon.apimgt.impl.kmclient.ApacheFeignHttpClient;
import org.wso2.carbon.apimgt.impl.utils.APIUtil;

import org.wso2.kong.client.model.KongPlugin;
import org.wso2.kong.client.model.KongRoute;
import org.wso2.kong.client.model.KongService;
import org.wso2.kong.client.model.PagedResponse;
import org.wso2.kong.client.util.KongAPIUtil;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

/**
 * This class controls the API artifact deployments on the Kong Gateway.
 */
public class KongGatewayDeployer implements GatewayDeployer {
    private static final Log log = LogFactory.getLog(KongGatewayDeployer.class);
    private Environment environment;
    private KongKonnectApi apiGatewayClient;
    private String controlPlaneId;
    private String authToken;

    @Override
    public void init(Environment environment) throws APIManagementException {
        this.environment = environment;
        if (!KongAPIUtil.isKubernetesDeployment(environment)) {
            // TODO: Need to add support for non kubernetes deployments
        }
        log.debug("Initializing KongConnect Gateway Deployer for environment: " + environment.getName());
        Map<String, String> properties = environment.getAdditionalProperties();
        if (properties == null) {
            throw new APIManagementException("Missing environment additionalProperties for Kong configuration");
        }
        try {
            this.environment = environment;
            String adminURL = properties.get(KongConstants.KONG_ADMIN_URL);
            this.controlPlaneId = properties.get(KongConstants.KONG_CONTROL_PLANE_ID);
            this.authToken = properties.get(KongConstants.KONG_AUTH_TOKEN);

            if (adminURL == null || controlPlaneId == null || authToken == null) {
                throw new APIManagementException("Missing required Kong environment configurations");
            }
            // Build Apache HttpClient (add timeouts/SSL as needed)
            HttpClient httpClient = APIUtil.getHttpClient(adminURL);

            // Bearer token interceptor
            RequestInterceptor auth = template ->
                    template.header("Authorization", "Bearer " + authToken);
            apiGatewayClient = Feign.builder()
                    .client(new ApacheFeignHttpClient(httpClient))
                    .options(new Request.Options(10000, 30000))
                    .encoder(new GsonEncoder())
                    .decoder(new GsonDecoder())
                    .errorDecoder(new KongErrorDecoder())
                    .logger(new Slf4jLogger(KongKonnectApi.class))
                    .requestInterceptor(auth)
                    .target(KongKonnectApi.class, adminURL);
            log.debug("Initialization completed Kong Gateway Deployer for environment: " + environment.getName());
        } catch (Exception e) {
            throw new APIManagementException("Error occurred while initializing Kong Gateway Deployer", e);
        }
    }

    @Override
    public String getType() {
        return KongConstants.KONG_TYPE;
    }

    @Override
    public String deploy(API api, String externalReference) throws APIManagementException {
        if (api != null) {
            if (KongAPIUtil.isKubernetesDeployment(environment)) {
                return KongAPIUtil.buildEndpointConfigJsonForKubernetes(api, environment);
            }
            KongService service = KongAPIUtil.buildKongService(api);
            try {
                if (StringUtils.isEmpty(externalReference)) {
                    KongService createdService = apiGatewayClient.createService(controlPlaneId, service);
                    if (createdService != null) {
                        // Create routes for the service
                        List<KongRoute> kongRoutes = KongAPIUtil.buildKongRoutes(api, createdService.getId());
                        for (KongRoute route : kongRoutes) {
                            try {
                                KongRoute createdRoute =
                                        apiGatewayClient.createRouteForService(controlPlaneId, createdService.getId(),
                                                route);
                            } catch (KongGatewayException e) {
                                throw new APIManagementException("Error creating route for API " + api.getId(), e);
                            }
                        }
                        List<KongPlugin> kongPlugins =
                                KongAPIUtil.buildKongPluginsForService(api, createdService.getId());
                        for (KongPlugin kongPlugin : kongPlugins) {
                            KongPlugin createdPlugin =
                                    apiGatewayClient.createPluginForService(controlPlaneId, createdService.getId(),
                                            kongPlugin);
                        }
                        return createdService.getId();
                    } else {
                        throw new APIManagementException("Failed to create service for API " + api.getId());
                    }
                } else {
                    KongService retrieveService = apiGatewayClient.retrieveService(controlPlaneId, externalReference);
                    if (retrieveService != null) {
                        // Update existing service
                        KongService updatedService =
                                apiGatewayClient.upsertService(controlPlaneId, externalReference, service);
                        if (updatedService != null) {
                            updateRoutes(api, externalReference, updatedService, service);
                            updateKongPluginsAtServiceLevel(api, externalReference, service);
                            return externalReference;
                        } else {
                            throw new APIManagementException("Failed to update service for API " + api.getId());
                        }
                    } else {
                        throw new APIManagementException(
                                "Service not found for external reference " + externalReference);
                    }
                }
            } catch (KongGatewayException e) {
                throw new APIManagementException("Error creating service for API " + api.getId(), e);
            }
        }
        throw new APIManagementException("API cannot be null for deployment");
    }

    private void updateKongPluginsAtServiceLevel(API api, String externalReference,
                                                 KongService service) throws APIManagementException {
        // Create or update routes for the service
        List<KongPlugin> kongPlugins = KongAPIUtil.buildKongPluginsForService(api, externalReference);
        PagedResponse<KongPlugin> kongPluginPagedResponse;
        try {
            kongPluginPagedResponse = apiGatewayClient.listPluginsByServiceId(controlPlaneId, externalReference, 1000);
        } catch (KongGatewayException e) {
            if (e.getStatusCode() == 404) {
                kongPluginPagedResponse = new PagedResponse<>(Collections.emptyList(), null, null);
            } else {
                throw new APIManagementException("Error retrieving plugins for service " + service.getId(), e);
            }
        }
        List<KongPlugin> existingPlugins = kongPluginPagedResponse.getData();
        List<KongPlugin> newlyAddedPlugins = new ArrayList<>(kongPlugins);
        newlyAddedPlugins.removeAll(existingPlugins);

        List<KongPlugin> modifiedPlugins = new ArrayList<>(kongPlugins);
        modifiedPlugins.retainAll(existingPlugins);
        for (KongPlugin modifiedPlugin : modifiedPlugins) {
            for (KongPlugin existingPlugin : existingPlugins) {
                if (existingPlugin.equals(modifiedPlugin)) {
                    // Set the ID of the existing plugin to the modified plugin for accurate comparison during upsert
                    modifiedPlugin.setId(existingPlugin.getId());
                    break;
                }
            }
        }
        List<KongPlugin> removedPlugins = new ArrayList<>(existingPlugins);
        removedPlugins.removeAll(kongPlugins);
        for (KongPlugin kongPlugin : newlyAddedPlugins) {
            try {
                KongPlugin createdKongPlugin =
                        apiGatewayClient.createPluginForService(controlPlaneId, externalReference, kongPlugin);
            } catch (KongGatewayException e) {
                throw new APIManagementException("Error creating plugin for API " + api.getId(), e);
            }
        }
        for (KongPlugin kongPlugin : modifiedPlugins) {
            try {
                KongPlugin updatedRoute =
                        apiGatewayClient.upsertPluginsAssociatedWithService(controlPlaneId, externalReference,
                                kongPlugin.getId(), kongPlugin);
            } catch (KongGatewayException e) {
                throw new APIManagementException("Error updating plugin for API " + api.getId(), e);
            }
        }
        for (KongPlugin kongPlugin : removedPlugins) {
            try {
                apiGatewayClient.deletePluginAssociatedWithService(controlPlaneId, externalReference,
                        kongPlugin.getId());
            } catch (KongGatewayException e) {
                throw new APIManagementException(
                        "Error deleting plugin " + kongPlugin.getId() + " for service " +
                                externalReference, e);
            }
        }
    }

    private void updateRoutes(API api, String externalReference, KongService updatedService, KongService service)
            throws APIManagementException {
        // Create or update routes for the service
        try {
            List<KongRoute> kongRoutes = KongAPIUtil.buildKongRoutes(api, updatedService.getId());
            PagedResponse<KongRoute> kongRoutePagedResponse =
                    apiGatewayClient.listRoutesByServiceId(controlPlaneId, externalReference, 1000);
            List<KongRoute> existingRoutes = kongRoutePagedResponse.getData();
            List<KongRoute> newlyAddedRoutes = new ArrayList<>(kongRoutes);
            newlyAddedRoutes.removeAll(existingRoutes);

            List<KongRoute> modifiedRoutes = new ArrayList<>(kongRoutes);
            modifiedRoutes.retainAll(existingRoutes);
            for (KongRoute modifiedRoute : modifiedRoutes) {
                for (KongRoute existingRoute : existingRoutes) {
                    if (existingRoute.equals(modifiedRoute)) {
                        // Set the ID of the existing route to the modified route for accurate comparison during upsert
                        modifiedRoute.setId(existingRoute.getId());
                        modifiedRoute.setProtocols(existingRoute.getProtocols());
                        break;
                    }
                }
            }
            List<KongRoute> removedRoutes = new ArrayList<>(existingRoutes);
            removedRoutes.removeAll(kongRoutes);
            for (KongRoute route : newlyAddedRoutes) {
                try {
                    KongRoute createdRoute =
                            apiGatewayClient.createRouteForService(controlPlaneId,
                                    externalReference,
                                    route);
                } catch (KongGatewayException e) {
                    throw new APIManagementException("Error creating route for API " + api.getId(), e);
                }
            }
            for (KongRoute route : modifiedRoutes) {
                try {
                    KongRoute updatedRoute =
                            apiGatewayClient.upsertRouteAssociatedWithService(controlPlaneId,
                                    externalReference, route.getId(), route);
                } catch (KongGatewayException e) {
                    throw new APIManagementException("Error updating route for API " + api.getId(), e);
                }
            }
            for (KongRoute route : removedRoutes) {
                try {
                    apiGatewayClient.deleteRouteAssociatedWithService(controlPlaneId, externalReference,
                            route.getId());
                } catch (KongGatewayException e) {
                    throw new APIManagementException(
                            "Error deleting route " + route.getId() + " for service " +
                                    externalReference, e);
                }
            }
        } catch (KongGatewayException e) {
            throw new APIManagementException("Error retrieving routes for service " + service.getId(), e);
        }
    }

    @Override
    public boolean undeploy(String s) throws APIManagementException {
        return true;
    }

    @Override
    public boolean undeploy(String externalReference, boolean delete) throws APIManagementException {
        if (KongAPIUtil.isKubernetesDeployment(environment)) {
            return true;
        }
        if (delete) {
            // API got deleted from APIM end.
            PagedResponse<KongRoute> kongRoutePagedResponse =
                    null;
            try {
                kongRoutePagedResponse =
                        apiGatewayClient.listRoutesByServiceId(controlPlaneId, externalReference, 1000);
            } catch (KongGatewayException e) {
                if (e.getStatusCode() == 404) {
                    log.warn("Service with id " + externalReference + " not found in Kong");
                    return true;
                } else {
                    throw new APIManagementException("Error retrieving routes for service " + externalReference, e);
                }
            }
            List<KongRoute> routeList = kongRoutePagedResponse.getData();
            if (routeList != null && !routeList.isEmpty()) {
                for (KongRoute route : routeList) {
                    try {
                        apiGatewayClient.deleteRouteAssociatedWithService(controlPlaneId, externalReference,
                                route.getId());
                    } catch (KongGatewayException e) {
                        throw new APIManagementException(
                                "Error deleting route " + route.getId() + " for service " + externalReference, e);
                    }
                }
            }
            try {
                apiGatewayClient.deleteService(controlPlaneId, externalReference);
            } catch (KongGatewayException e) {
                throw new APIManagementException("Error deleting service " + externalReference, e);
            }
        } else {
            // API Revision got deployed from APIM end. Do nothing as of now.
            try {
                KongService service = apiGatewayClient.retrieveService(controlPlaneId, externalReference);
                service.setEnabled(false);
                apiGatewayClient.upsertService(controlPlaneId, externalReference, service);
            } catch (KongGatewayException e) {
                if (e.getStatusCode() == 404) {
                    log.warn("Service with id " + externalReference + " not found in Kong");
                } else {
                    throw new APIManagementException("Error disabling kong service " + externalReference, e);
                }
            }
        }
        return true;
    }

    @Override
    public GatewayAPIValidationResult validateApi(API api) throws APIManagementException {
        GatewayAPIValidationResult gatewayAPIValidationResult = new GatewayAPIValidationResult();
        if (KongAPIUtil.isKubernetesDeployment(environment)) {
            gatewayAPIValidationResult.setValid(true);
            gatewayAPIValidationResult.setErrors(Collections.<String>emptyList());
        } else {
            // TODO: Need to add support for non kubernetes deployments
            gatewayAPIValidationResult.setValid(true);
            gatewayAPIValidationResult.setErrors(Collections.<String>emptyList());
        }
        return gatewayAPIValidationResult;
    }

    @Override
    public String getAPIExecutionURL(String externalReference) throws APIManagementException {
        if (KongAPIUtil.isKubernetesDeployment(environment)) {
            return KongAPIUtil.getAPIExecutionURLForKubernetes(externalReference, null);
        }
        String vhost = environment.getVhosts() != null && !environment.getVhosts().isEmpty()
                ? environment.getVhosts().get(0).getHost() : KongConstants.DEFAULT_VHOST;
        return "https://" + vhost;
    }

    @Override
    public String getAPIExecutionURL(String externalReference, HttpScheme httpScheme) throws APIManagementException {
        if (KongAPIUtil.isKubernetesDeployment(environment)) {
            String protocol = (httpScheme == HttpScheme.HTTP) ?
                    KongConstants.HTTP_PROTOCOL :
                    KongConstants.HTTPS_PROTOCOL;
            return KongAPIUtil.getAPIExecutionURLForKubernetes(externalReference, protocol);
        }

        // For standalone mode, maintain backward compatibility by calling the legacy method
        return getAPIExecutionURL(externalReference);
    }

    @Override
    public void transformAPI(API api) throws APIManagementException {
        if (!KongAPIUtil.isKubernetesDeployment(environment)) {
            // TODO: Need to add support for non kubernetes deployments
            return;
        }
    }
}
