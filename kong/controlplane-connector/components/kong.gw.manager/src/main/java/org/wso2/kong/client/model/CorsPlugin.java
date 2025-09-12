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


package org.wso2.kong.client.model;

import com.google.gson.annotations.SerializedName;
import java.util.Arrays;
import java.util.List;

/**
 * Represents a CORS plugin configuration for Kong API Gateway.
 */
public class CorsPlugin {
    @SerializedName("allow_origin_absent")
    private boolean allowConfigAbsent = true;
    @SerializedName("credentials")
    private boolean enableCredentials = false;
    @SerializedName("exposed_headers")
    private List<String> exposeHeaders;
    @SerializedName("headers")
    private List<String> allowHeaders;
    @SerializedName("max_age")
    private int maxAge;
    @SerializedName("methods")
    private List<String> allowMethods = Arrays.asList("CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST",
            "PUT", "TRACE");
    @SerializedName("origins")
    private List<String> allowOrigins = Arrays.asList("*");
    @SerializedName("preflight_continue")
    private boolean preflightContinue = false;
    @SerializedName("private_network")
    private boolean privateNetworkAccess = false;

    /**
     * @param allowConfigAbsent
     * @param enableCredentials
     * @param exposeHeaders
     * @param allowHeaders
     * @param allowMethods
     * @param allowOrigins
     */
    public CorsPlugin(boolean allowConfigAbsent, boolean enableCredentials, List<String> exposeHeaders,
                      List<String> allowHeaders, List<String> allowMethods, List<String> allowOrigins) {
        this.allowConfigAbsent = allowConfigAbsent;
        this.enableCredentials = enableCredentials;
        this.exposeHeaders = exposeHeaders;
        this.allowHeaders = allowHeaders;
        this.allowMethods = allowMethods;
        this.allowOrigins = allowOrigins;
    }

    public boolean isAllowConfigAbsent() {
        return allowConfigAbsent;
    }

    public void setAllowConfigAbsent(boolean allowConfigAbsent) {
        this.allowConfigAbsent = allowConfigAbsent;
    }

    public boolean isEnableCredentials() {
        return enableCredentials;
    }

    public void setEnableCredentials(boolean enableCredentials) {
        this.enableCredentials = enableCredentials;
    }

    public List<String> getExposeHeaders() {
        return exposeHeaders;
    }

    public void setExposeHeaders(List<String> exposeHeaders) {
        this.exposeHeaders = exposeHeaders;
    }

    public List<String> getAllowHeaders() {
        return allowHeaders;
    }

    public void setAllowHeaders(List<String> allowHeaders) {
        this.allowHeaders = allowHeaders;
    }

    public int getMaxAge() {
        return maxAge;
    }

    public void setMaxAge(int maxAge) {
        this.maxAge = maxAge;
    }

    public List<String> getAllowMethods() {
        return allowMethods;
    }

    public void setAllowMethods(List<String> allowMethods) {
        this.allowMethods = allowMethods;
    }

    public List<String> getAllowOrigins() {
        return allowOrigins;
    }

    public void setAllowOrigins(List<String> allowOrigins) {
        this.allowOrigins = allowOrigins;
    }

    public boolean isPreflightContinue() {
        return preflightContinue;
    }

    public void setPreflightContinue(boolean preflightContinue) {
        this.preflightContinue = preflightContinue;
    }

    public boolean isPrivateNetworkAccess() {
        return privateNetworkAccess;
    }

    public void setPrivateNetworkAccess(boolean privateNetworkAccess) {
        this.privateNetworkAccess = privateNetworkAccess;
    }
}
