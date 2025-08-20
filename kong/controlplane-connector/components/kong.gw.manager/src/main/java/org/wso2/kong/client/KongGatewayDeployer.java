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
import feign.RequestInterceptor;
import feign.gson.GsonDecoder;
import feign.gson.GsonEncoder;
import feign.slf4j.Slf4jLogger;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import org.apache.http.impl.client.CloseableHttpClient;
import org.apache.http.impl.client.HttpClients;
import org.wso2.carbon.apimgt.api.APIManagementException;
import org.wso2.carbon.apimgt.api.model.API;
import org.wso2.carbon.apimgt.api.model.Environment;
import org.wso2.carbon.apimgt.api.model.GatewayAPIValidationResult;
import org.wso2.carbon.apimgt.api.model.GatewayDeployer;
import org.wso2.carbon.apimgt.impl.kmclient.ApacheFeignHttpClient;
import org.wso2.kong.client.model.KongRoute;
import org.wso2.kong.client.model.KongService;
import org.wso2.kong.client.model.PagedResponse;
import org.wso2.kong.client.util.KongAPIUtil;
import java.util.ArrayList;
import org.wso2.kong.client.util.KongAPIUtil;

import java.util.Collections;
import java.util.List;

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
        log.debug("Initializing AWS Gateway Deployer for environment: " + environment.getName());
        try {
            this.environment = environment;
            String adminURL = environment.getAdditionalProperties().get(KongConstants.KONG_ADMIN_URL);
            this.controlPlaneId = environment.getAdditionalProperties().get(KongConstants.KONG_CONTROL_PLANE_ID);
            this.authToken = environment.getAdditionalProperties().get(KongConstants.KONG_AUTH_TOKEN);

            if (adminURL == null || controlPlaneId == null || authToken == null) {
                throw new APIManagementException("Missing required Kong environment configurations");
            }
            // Build Apache HttpClient (add timeouts/SSL as needed)
            CloseableHttpClient httpClient = HttpClients.custom().build();

            // Bearer token interceptor
            RequestInterceptor auth = template ->
                    template.header("Authorization", "Bearer " + authToken);
            apiGatewayClient = Feign.builder()
                    .client(new ApacheFeignHttpClient(httpClient))
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
                if (externalReference == null) {
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
                            // Create or update routes for the service
                            List<KongRoute> kongRoutes = KongAPIUtil.buildKongRoutes(api, updatedService.getId());
                            PagedResponse<KongRoute> kongRoutePagedResponse =
                                    apiGatewayClient.listRoutesByServiceId(controlPlaneId, service.getId(), 1000);
                            List<KongRoute> existingRoutes = kongRoutePagedResponse.getData();
                            List<KongRoute> newlyAddedRoutes = new ArrayList<>(kongRoutes);
                            newlyAddedRoutes.removeAll(existingRoutes);

                            List<KongRoute> modifiedRoutes = new ArrayList<>(kongRoutes);
                            modifiedRoutes.retainAll(existingRoutes);

                            List<KongRoute> removedRoutes = new ArrayList<>(existingRoutes);
                            removedRoutes.removeAll(kongRoutes);
                            for (KongRoute route : newlyAddedRoutes) {
                                try {
                                    KongRoute createdRoute =
                                            apiGatewayClient.createRouteForService(controlPlaneId,
                                                    updatedService.getId(),
                                                    route);
                                } catch (KongGatewayException e) {
                                    throw new APIManagementException("Error creating route for API " + api.getId(), e);
                                }
                            }
                            for (KongRoute route : modifiedRoutes) {
                                try {
                                    KongRoute updatedRoute =
                                            apiGatewayClient.upsertRouteAssociatedWithService(controlPlaneId,
                                                    updatedService.getId(), route.getId(), route);
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
            return service.getId();
        }
        throw new APIManagementException("API cannot be null for deployment");
    }

    @Override
    public boolean undeploy(String externalReference) throws APIManagementException {
        if (KongAPIUtil.isKubernetesDeployment(environment)) {
            return true;
        }
        PagedResponse<KongRoute> kongRoutePagedResponse =
                apiGatewayClient.listRoutesByServiceId(controlPlaneId, externalReference, 1000);
        List<KongRoute> routeList = kongRoutePagedResponse.getData();
        if (routeList != null && !routeList.isEmpty()) {
            for (KongRoute route : routeList) {
                try {
                    apiGatewayClient.deleteRouteAssociatedWithService(controlPlaneId, externalReference, route.getId());
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
            gatewayAPIValidationResult.setValid(false);
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
